package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tidwall/gjson"
)

// Batch shares the native upper bound without importing relay/helper, which
// already depends on service.
const maxBatchLineTokensBound = int64(common.MaxRequestTokens)

// BatchFileParseResult summarizes one validated JSONL pass.
type BatchFileParseResult struct {
	LineCount int64
	Model     string
	Checksum  string
	SizeBytes int64
}

type batchLineRecord struct {
	CustomId string
	Body     []byte
}

// parseBatchLine validates one JSONL record shape. It returns the extracted
// fields without echoing any request body text in errors.
func parseBatchLine(line []byte, lineNumber int) (*batchLineRecord, *dto.BatchValidateError) {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d is empty", lineNumber)}
	}
	if !gjson.ValidBytes(line) {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d is not valid JSON", lineNumber)}
	}
	if batchJSONHasDuplicateKeys(gjson.ParseBytes(line)) {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: "batch JSON contains duplicate object fields"}
	}
	customId := gjson.GetBytes(line, "custom_id")
	if !customId.Exists() || customId.Type != gjson.String || strings.TrimSpace(customId.String()) == "" {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d must set a non-empty string custom_id", lineNumber)}
	}
	if len(customId.String()) > 255 {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d custom_id must not exceed 255 characters", lineNumber)}
	}
	method := gjson.GetBytes(line, "method")
	if !method.Exists() || !strings.EqualFold(method.String(), "POST") {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d method must be POST", lineNumber)}
	}
	url := gjson.GetBytes(line, "url")
	if !url.Exists() || url.String() != dto.BatchEndpointChatCompletions {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d url must be %s", lineNumber, dto.BatchEndpointChatCompletions)}
	}
	model := gjson.GetBytes(line, "body.model")
	if !model.Exists() || model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d body.model is required", lineNumber)}
	}
	body := gjson.GetBytes(line, "body")
	if !body.Exists() || body.Type != gjson.JSON {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d body must be a JSON object", lineNumber)}
	}
	if !body.Get("messages").IsArray() {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d body.messages must be an array", lineNumber)}
	}
	if stream := body.Get("stream"); stream.Exists() && stream.Type != gjson.False {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d body.stream=true is not supported for batch requests", lineNumber)}
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		if value := body.Get(key); value.Exists() {
			n, err := strconv.ParseInt(value.Raw, 10, 64)
			if value.Type != gjson.Number || err != nil || n <= 0 || n > maxBatchLineTokensBound {
				return nil, &dto.BatchValidateError{Line: lineNumber, Message: "output cap must be a bounded positive integer"}
			}
		}
	}
	for _, message := range body.Get("messages").Array() {
		content := message.Get("content")
		if content.IsArray() {
			for _, part := range content.Array() {
				if part.Get("type").String() != "text" || part.Get("text").Type != gjson.String {
					return nil, &dto.BatchValidateError{Line: lineNumber, Message: "batch messages support text content only"}
				}
			}
		} else if content.Type != gjson.String && content.Type != gjson.Null {
			return nil, &dto.BatchValidateError{Line: lineNumber, Message: "batch message content must be text"}
		}
	}
	if body.Get("audio").Exists() || body.Get("modalities").Exists() {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: "batch supports text completions only"}
	}
	maxTokens := body.Get("max_completion_tokens")
	if !maxTokens.Exists() {
		maxTokens = body.Get("max_tokens")
	}
	if !maxTokens.Exists() || maxTokens.Type != gjson.Number {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d must set a numeric output cap (max_completion_tokens or max_tokens)", lineNumber)}
	}
	outputCap, intErr := strconv.ParseInt(maxTokens.Raw, 10, 64)
	if intErr != nil || outputCap <= 0 || outputCap > maxBatchLineTokensBound {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d output cap must be between 1 and %d", lineNumber, maxBatchLineTokensBound)}
	}
	n := body.Get("n")
	nValue, nErr := strconv.ParseInt(n.Raw, 10, 64)
	if n.Exists() && (n.Type != gjson.Number || nErr != nil || nValue < 1 || nValue > 128) {
		return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d body.n is outside the supported range", lineNumber)}
	}
	return &batchLineRecord{CustomId: customId.String(), Body: []byte(body.Raw)}, nil
}

// ValidateBatchJSONL performs one full streaming pass over the input file.
// When strictModel is non-empty every line must carry exactly that model;
// otherwise all lines must agree on one model, which is returned. The whole
// batch is rejected on the first bad line, before any inference or billing.
func ValidateBatchJSONL(r io.Reader, strictModel string) (*BatchFileParseResult, error) {
	hasher := sha256.New()
	counting := &countingReader{r: r}
	scanner := bufio.NewScanner(counting)
	scanner.Buffer(make([]byte, 0, 64*1024), dto.MaxBatchLineBytes)

	seen := make(map[string]struct{})
	result := &BatchFileParseResult{}
	lineNumber := 0
	var totalLines int64
	for scanner.Scan() {
		lineNumber++
		totalLines++
		if totalLines > dto.MaxBatchRequestsPerFile {
			return nil, &dto.BatchValidateError{Line: lineNumber, Message: "batch exceeds the maximum of 10,000 requests per file"}
		}
		if counting.n > dto.MaxBatchFileBytes {
			return nil, &dto.BatchValidateError{Message: fmt.Sprintf("batch exceeds the maximum file size of %d bytes", dto.MaxBatchFileBytes)}
		}
		line := scanner.Bytes()
		hasher.Write(line)
		hasher.Write([]byte("\n"))
		record, parseErr := parseBatchLine(line, lineNumber)
		if parseErr != nil {
			return nil, parseErr
		}
		if _, duplicate := seen[record.CustomId]; duplicate {
			return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d repeats custom_id %q", lineNumber, record.CustomId)}
		}
		seen[record.CustomId] = struct{}{}
		lineModel := gjson.GetBytes(line, "body.model").String()
		if result.Model == "" {
			result.Model = lineModel
		} else if result.Model != lineModel {
			return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d switches the model; a batch must use a single model", lineNumber)}
		}
		if strictModel != "" && lineModel != strictModel {
			return nil, &dto.BatchValidateError{Line: lineNumber, Message: fmt.Sprintf("line %d model does not match the requested model", lineNumber)}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, &dto.BatchValidateError{Line: lineNumber + 1, Message: fmt.Sprintf("line %d exceeds the maximum line length", lineNumber+1)}
		}
		return nil, fmt.Errorf("failed to read batch file: %w", err)
	}
	if totalLines == 0 {
		return nil, &dto.BatchValidateError{Message: "batch file contains no requests"}
	}
	result.LineCount = totalLines
	result.SizeBytes = counting.n
	result.Checksum = hex.EncodeToString(hasher.Sum(nil))
	if strictModel != "" && result.Model != strictModel {
		return nil, &dto.BatchValidateError{Message: "batch file model does not match the requested model"}
	}
	return result, nil
}

// BatchLineEstimate carries one line's pessimistic pre-consume inputs.
type BatchLineEstimate struct {
	CustomId  string
	InputEst  int64
	OutputCap int64
}

// BatchConvertedFile is one validated create-time pass: the converted bytes
// for the upstream provider plus per-line estimates for the budget.
type BatchConvertedFile struct {
	Parse         *BatchFileParseResult
	Bytes         []byte
	LineEstimates []BatchLineEstimate
	LineInputs    map[string]int
}

// ConvertBatchJSONLForDeployment validates again and rewrites every line's
// body.model to the frozen upstream deployment name, collecting per-line
// budget estimates in the same pass. The converted bytes are only ever sent
// to the upstream provider.
func ConvertBatchJSONLForDeployment(data []byte, deployment string) (*BatchConvertedFile, error) {
	if int64(len(data)) > dto.MaxBatchFileBytes {
		return nil, &dto.BatchValidateError{Message: fmt.Sprintf("batch exceeds the maximum file size of %d bytes", dto.MaxBatchFileBytes)}
	}
	validateResult, err := ValidateBatchJSONL(bytes.NewReader(data), "")
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	estimates := make([]BatchLineEstimate, 0, validateResult.LineCount)
	lineInputs := make(map[string]int, validateResult.LineCount)
	lineNumber := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), dto.MaxBatchLineBytes)
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		record, parseErr := parseBatchLine(line, lineNumber)
		if parseErr != nil {
			return nil, parseErr
		}
		converted, convertErr := rewriteBatchLineModel(line, deployment)
		if convertErr != nil {
			return nil, convertErr
		}
		out.Write(converted)
		out.WriteByte('\n')
		outputCap := int64(0)
		if cap := gjson.GetBytes(line, "body.max_completion_tokens"); cap.Exists() {
			outputCap = cap.Int()
		} else if cap := gjson.GetBytes(line, "body.max_tokens"); cap.Exists() {
			outputCap = cap.Int()
		}
		if n := gjson.GetBytes(line, "body.n"); n.Exists() {
			outputCap *= n.Int()
		}
		inputEst := EstimateBatchLineInputTokens(int64(len(record.Body)))
		estimates = append(estimates, BatchLineEstimate{
			CustomId: record.CustomId, InputEst: inputEst, OutputCap: outputCap,
		})
		lineInputs[record.CustomId] = int(inputEst)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to convert batch file: %w", err)
	}
	validateResult.SizeBytes = int64(out.Len())
	return &BatchConvertedFile{
		Parse: validateResult, Bytes: out.Bytes(),
		LineEstimates: estimates, LineInputs: lineInputs,
	}, nil
}

// rewriteBatchLineModel rebuilds the line with body.model set to the frozen
// upstream deployment name. All other client bytes are carried over verbatim.
func rewriteBatchLineModel(line []byte, deployment string) ([]byte, *dto.BatchValidateError) {
	var value map[string]json.RawMessage
	if err := common.Unmarshal(line, &value); err != nil {
		return nil, &dto.BatchValidateError{Message: "invalid batch JSON"}
	}
	var body map[string]json.RawMessage
	if err := common.Unmarshal(value["body"], &body); err != nil {
		return nil, &dto.BatchValidateError{Message: "invalid batch body"}
	}
	body["model"], _ = common.Marshal(deployment)
	value["body"], _ = common.Marshal(body)
	converted, err := common.Marshal(value)
	if err != nil {
		return nil, &dto.BatchValidateError{Message: "batch conversion failed"}
	}
	return converted, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// Duplicate object keys have different meanings across JSON decoders. Reject
// them at the Batch input boundary before validation, freezing or conversion.
func batchJSONHasDuplicateKeys(value gjson.Result) bool {
	if !value.IsObject() && !value.IsArray() {
		return false
	}
	seen := map[string]bool{}
	duplicate := false
	value.ForEach(func(key, child gjson.Result) bool {
		if value.IsObject() {
			name := key.String()
			if seen[name] {
				duplicate = true
				return false
			}
			seen[name] = true
		}
		duplicate = batchJSONHasDuplicateKeys(child)
		return !duplicate
	})
	return duplicate
}
