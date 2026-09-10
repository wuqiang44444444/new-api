package azurebatch

import (
	"bufio"
	"bytes"
	"errors"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tidwall/gjson"
	"io"
	"strconv"
)

// LineResult is one parsed result-file row.
type LineResult struct {
	CustomId     string
	Status       string
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
	TotalTokens  int64
	ErrorCode    string
}

// Successful usage is nested under response.body. Missing or malformed usage
// is untrusted, never a zero-cost success.
func ParseResultLines(reader io.Reader) ([]LineResult, error) {
	results := make([]LineResult, 0)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), dto.MaxBatchLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if !gjson.ValidBytes(line) {
			return nil, errors.New("batch result contains invalid JSON")
		}
		value := gjson.ParseBytes(line)
		id := value.Get("custom_id")
		if id.Type != gjson.String || id.String() == "" {
			return nil, errors.New("batch result has no request identity")
		}
		result := LineResult{CustomId: id.String()}
		code := value.Get("response.status_code")
		if code.Raw == "200" {
			result.Status = "completed"
			usage := value.Get("response.body.usage")
			var err error
			result.InputTokens, err = batchUsageInteger(usage.Get("prompt_tokens"), true)
			if err != nil {
				return nil, err
			}
			result.OutputTokens, err = batchUsageInteger(usage.Get("completion_tokens"), true)
			if err != nil {
				return nil, err
			}
			result.TotalTokens, err = batchUsageInteger(usage.Get("total_tokens"), true)
			if err != nil {
				return nil, err
			}
			result.CachedTokens, err = batchUsageInteger(usage.Get("prompt_tokens_details.cached_tokens"), false)
			if err != nil {
				return nil, err
			}
			if result.CachedTokens > result.InputTokens || result.TotalTokens != result.InputTokens+result.OutputTokens {
				return nil, errors.New("batch usage is inconsistent")
			}
		} else {
			if usage := value.Get("response.body.usage"); usage.Exists() && usage.Type != gjson.Null {
				return nil, errors.New("batch failure contains ambiguous usage; reconciliation required")
			}
			errorFact := value.Get("error")
			statusCode, _ := strconv.ParseInt(code.Raw, 10, 64)
			if !errorFact.IsObject() && (statusCode < 400 || statusCode > 599) {
				return nil, errors.New("batch failure evidence is missing")
			}
			result.Status = "failed"
			result.ErrorCode = "request_failed"
		}
		results = append(results, result)
		if len(results) > dto.MaxBatchRequestsPerFile {
			return nil, errors.New("batch result contains too many requests")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("batch result could not be read completely")
	}
	return results, nil
}

func batchUsageInteger(value gjson.Result, required bool) (int64, error) {
	if !required && !value.Exists() {
		return 0, nil
	}
	number, err := strconv.ParseInt(value.Raw, 10, 64)
	if value.Type != gjson.Number || err != nil || number < 0 || number > 1<<53-1 {
		return 0, errors.New("batch usage is missing or outside the supported range")
	}
	return number, nil
}
