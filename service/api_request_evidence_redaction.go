package service

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// A damaged JSON/SSE frame is still evidence. Scan quoted keys independently
// of the document grammar so credential values are removed before retaining a
// malformed or truncated prefix. An unfinished credential consumes the suffix.
func redactAPIObservedBody(body []byte, contentType string) ([]byte, error) {
	payload, err := evidenceRedactBody(body, contentType)
	if err == nil {
		if utf8.Valid(payload) {
			payload = []byte(serviceMaskSignedURLs(string(payload)))
		}
		return payload, nil
	}
	if !strings.Contains(contentType, "json") && !strings.Contains(contentType, "event-stream") {
		return nil, err // Multipart boundaries and binary formats must not be guessed.
	}
	var out bytes.Buffer
	for i := 0; i < len(body); {
		if body[i] != '"' {
			out.WriteByte(body[i])
			i++
			continue
		}
		start := i
		i = apiQuotedEnd(body, i)
		if i > len(body) {
			out.Write(body[start:])
			break
		}
		token := body[start:i]
		var key string
		_ = common.Unmarshal(token, &key)
		next := i
		for next < len(body) && (body[next] == ' ' || body[next] == '\n' || body[next] == '\r' || body[next] == '\t') {
			next++
		}
		out.Write(token)
		if next >= len(body) || body[next] != ':' || !isEvidenceCredentialKey(key) {
			continue
		}
		out.Write(body[i : next+1])
		i = next + 1
		for i < len(body) && (body[i] == ' ' || body[i] == '\n' || body[i] == '\r' || body[i] == '\t') {
			i++
		}
		out.WriteString(`"[REDACTED]"`)
		depth := 0
		for i < len(body) {
			switch body[i] {
			case '"':
				i = min(apiQuotedEnd(body, i), len(body))
				continue
			case '{', '[':
				depth++
			case '}', ']':
				if depth == 0 {
					goto valueDone
				}
				depth--
			case ',', '\n':
				if depth == 0 {
					goto valueDone
				}
			}
			i++
		}
	valueDone:
	}
	return out.Bytes(), nil
}

func apiQuotedEnd(body []byte, start int) int {
	for i := start + 1; i < len(body); i++ {
		if body[i] == '\\' {
			i++
			continue
		}
		if body[i] == '"' {
			return i + 1
		}
	}
	return len(body) + 1
}

func (c *ImageErrorEvidence) redactObservedBody(body []byte, contentType string) ([]byte, error) {
	payload, err := redactAPIObservedBody(body, contentType)
	if err != nil || !utf8.Valid(payload) {
		return payload, err
	}
	trimmed := bytes.TrimSpace(payload)
	structured := strings.Contains(contentType, "json") || strings.Contains(contentType, "event-stream") || bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))
	if structured || strings.Contains(contentType, "text/") {
		payload = evidenceJSONStringPattern.ReplaceAllFunc(payload, func(token []byte) []byte {
			var value string
			if common.Unmarshal(token, &value) != nil {
				return token
			}
			if c.suppressSourceURLs {
				value = evidenceURLPattern.ReplaceAllString(value, evidenceRedactedPlaceholder)
			} else {
				value = serviceMaskSignedURLs(value)
				for _, secret := range c.secrets {
					if value == secret {
						value = evidenceRedactedPlaceholder
						break
					}
					if len(secret) >= 8 {
						value = strings.ReplaceAll(value, secret, evidenceRedactedPlaceholder)
					}
				}
			}
			masked, marshalErr := common.Marshal(value)
			if marshalErr != nil {
				return []byte(`"[REDACTED]"`)
			}
			return masked
		})
	}
	if !structured && strings.Contains(contentType, "text/") {
		for _, secret := range c.secrets {
			if len(secret) >= 8 {
				payload = bytes.ReplaceAll(payload, []byte(secret), []byte(evidenceRedactedPlaceholder))
			} else if bytes.Equal(payload, []byte(secret)) {
				payload = []byte(evidenceRedactedPlaceholder)
			}
		}
	}
	if c.suppressSourceURLs {
		payload = evidenceURLPattern.ReplaceAll(payload, []byte(evidenceRedactedPlaceholder))
	}
	return payload, nil
}
