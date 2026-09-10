package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// writeBatchPublicResult projects protocol metadata only. Answer content,
// custom_id and usage remain the customer's result, not text to sanitize.
// It runs before attaching the result object so file bytes and size agree.
func writeBatchPublicResult(dst io.Writer, src io.Reader, publicModel string) error {
	modelJSON, err := common.Marshal(publicModel)
	if err != nil {
		return err
	}
	publicError, err := common.Marshal(map[string]string{
		"code": "request_failed", "message": "The batch request failed",
	})
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), dto.MaxBatchLineBytes)
	for scanner.Scan() {
		var line map[string]json.RawMessage
		if err := common.Unmarshal(scanner.Bytes(), &line); err != nil {
			return fmt.Errorf("decode batch result: %w", err)
		}
		var response map[string]json.RawMessage
		if raw := line["response"]; len(raw) != 0 {
			if err := common.Unmarshal(raw, &response); err != nil {
				return fmt.Errorf("decode batch response: %w", err)
			}
		}
		var body map[string]json.RawMessage
		var status int
		if raw := response["status_code"]; len(raw) != 0 {
			if err := common.Unmarshal(raw, &status); err != nil {
				return fmt.Errorf("decode batch response status: %w", err)
			}
		}
		if response != nil && status != 200 {
			// Failure bodies are provider diagnostics, not chat completions.
			// Their shape must not prevent delivery of already trusted failures.
			body = map[string]json.RawMessage{"error": publicError}
		} else if raw := response["body"]; len(raw) != 0 {
			if err := common.Unmarshal(raw, &body); err != nil {
				return fmt.Errorf("decode batch response body: %w", err)
			}
		}
		if _, exists := body["model"]; exists {
			body["model"] = modelJSON
		}
		if raw := body["error"]; len(raw) != 0 && string(raw) != "null" {
			body["error"] = publicError
		}
		if body != nil {
			response["body"], err = common.Marshal(body)
			if err != nil {
				return err
			}
		}
		if response != nil {
			line["response"], err = common.Marshal(response)
			if err != nil {
				return err
			}
		}
		if raw := line["error"]; len(raw) != 0 && string(raw) != "null" {
			line["error"] = publicError
		}
		encoded, err := common.Marshal(line)
		if err != nil {
			return err
		}
		if _, err := dst.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	return scanner.Err()
}
