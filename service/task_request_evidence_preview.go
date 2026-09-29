package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var evidenceJSONStringPattern = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)

// evidenceMaskBodyURLs decodes the recorded wire format before masking URLs.
// It changes only the preview; malformed structured bodies never fall back to
// their original bytes. JSON number spellings are preserved for diagnostic IDs.
func evidenceMaskBodyURLs(body []byte, contentType string) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return nil, err
	}
	if len(body) == 0 {
		return body, nil
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		var out bytes.Buffer
		writer := multipart.NewWriter(&out)
		if err := writer.SetBoundary(params["boundary"]); err != nil {
			return nil, err
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			payload, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			partType := part.Header.Get("Content-Type")
			// Uploaded files have a download path; do not interpret binary data
			// or embedded metadata as text in the multipart preview.
			if part.FileName() != "" {
				payload = []byte(evidenceRedactedPlaceholder)
			} else {
				payload, err = evidenceMaskBodyURLs(payload, partType)
				if err != nil {
					return nil, err
				}
			}
			for key, values := range part.Header {
				for i, value := range values {
					part.Header[key][i] = serviceMaskSignedURLs(value)
				}
			}
			dest, err := writer.CreatePart(part.Header)
			if err != nil {
				return nil, err
			}
			if _, err := dest.Write(payload); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	if mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		masked := make(url.Values, len(values))
		for key, entries := range values {
			for _, value := range entries {
				key := serviceMaskSignedURLs(key)
				masked.Add(key, serviceMaskSignedURLs(value))
			}
		}
		return []byte(masked.Encode()), nil
	}
	if mediaType == "text/event-stream" {
		lines := bytes.Split(body, []byte("\n"))
		for i, line := range lines {
			if !bytes.HasPrefix(line, []byte("data:")) {
				lines[i] = []byte(serviceMaskSignedURLs(string(line)))
				continue
			}
			value := bytes.TrimSpace(line[5:])
			if len(value) == 0 || string(value) == "[DONE]" {
				continue
			}
			masked, err := evidenceMaskBodyURLs(value, "application/json")
			if err != nil {
				return nil, err
			}
			lines[i] = append([]byte("data: "), masked...)
		}
		return bytes.Join(lines, []byte("\n")), nil
	}
	trimmed := bytes.TrimSpace(body)
	if strings.Contains(mediaType, "json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')) {
		var raw json.RawMessage
		if err := common.Unmarshal(trimmed, &raw); err != nil {
			return nil, err
		}
		return evidenceJSONStringPattern.ReplaceAllFunc(raw, func(token []byte) []byte {
			// Validated JSON string tokens always decode/marshal successfully.
			var value string
			_ = common.Unmarshal(token, &value)
			masked, _ := common.Marshal(serviceMaskSignedURLs(value))
			return masked
		}), nil
	}
	return []byte(serviceMaskSignedURLs(string(body))), nil
}
