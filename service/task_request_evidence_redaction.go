package service

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// 证据脱敏：Authorization、Cookie、API Key、支付凭据和代理认证一律不保存，
// 使用占位符保持字段存在性。业务参数（含 max_tokens 等含 token 字样的
// 合法字段）必须保留。脱敏副本不是字节完全相同的原文，记录中明确标识。
const evidenceRedactedPlaceholder = "[REDACTED]"

// evidenceCredentialHeaderNames 是按小写精确匹配的认证头集合。
var evidenceCredentialHeaderNames = map[string]struct{}{
	"authorization":        {},
	"proxy-authorization":  {},
	"cookie":               {},
	"set-cookie":           {},
	"www-authenticate":     {},
	"x-api-key":            {},
	"api-key":              {},
	"apikey":               {},
	"x-auth-token":         {},
	"x-access-token":       {},
	"x-session-token":      {},
	"openai-organization":  {},
	"openai-project":       {},
	"anthropic-api-key":    {},
	"x-goog-api-key":       {},
	"x-amz-authorization":  {},
	"authorization-bearer": {},
	"new-api-user":         {},
	"payment-credential":   {},
	"proxy-connection":     {},
}

// evidenceCredentialJSONKeyNames 是 JSON 正文中按小写精确匹配的凭据键。
// 不使用宽泛的 *token* 子串匹配，避免误删 max_tokens 等业务字段。
var evidenceCredentialJSONKeyNames = map[string]struct{}{
	"token":               {},
	"authorization":       {},
	"proxy_authorization": {},
	"cookie":              {},
	"set_cookie":          {},
	"api_key":             {},
	"apikey":              {},
	"api-key":             {},
	"access_key":          {},
	"secret_key":          {},
	"secretaccesskey":     {},
	"accesskeyid":         {},
	"session_key":         {},
	"client_secret":       {},
	"private_key":         {},
	"password":            {},
	"passwd":              {},
	"credential":          {},
	"credentials":         {},
	"signature":           {},
	"x-goog-api-key":      {},
}

// evidenceCredentialJSONKeySuffixes 是按小写后缀匹配的凭据键模式。
// 只收敛到明确的 *_token / *_secret / *_password / *_api_key 族，
// max_tokens、prompt_tokens 等复数业务字段不会被命中。
var evidenceCredentialJSONKeySuffixes = []string{
	"_api_key", "_apikey", "_secret", "_password", "_credential", "_token",
}

// EvidenceRedactHeaders 对 http.Header 摘要脱敏后返回普通 map。
func EvidenceRedactHeaders(headers map[string][]string) map[string]string {
	result := make(map[string]string, len(headers))
	for name, values := range headers {
		if _, credential := evidenceCredentialHeaderNames[strings.ToLower(name)]; credential {
			result[name] = evidenceRedactedPlaceholder
			continue
		}
		result[name] = strings.Join(values, ", ")
	}
	return result
}

// EvidenceRedactQueryParams 对查询串中命名的凭据参数脱敏。
func EvidenceRedactQueryParams(query url.Values) map[string]string {
	result := make(map[string]string, len(query))
	for key, values := range query {
		if isEvidenceCredentialKey(key) {
			result[key] = evidenceRedactedPlaceholder
			continue
		}
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func isEvidenceCredentialKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if _, exact := evidenceCredentialJSONKeyNames[lower]; exact {
		return true
	}
	for _, suffix := range evidenceCredentialJSONKeySuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func evidenceRedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isEvidenceCredentialKey(key) {
				result[key] = evidenceRedactedPlaceholder
				continue
			}
			result[key] = evidenceRedactValue(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = evidenceRedactValue(child)
		}
		return result
	default:
		return value
	}
}

// EvidenceMaskSignedURLs 是展示层遮盖：把常见签名参数值替换为省略号。
// 存储正文仍是完整业务内容（受权限保护），遮盖只发生在查询视图。
func EvidenceMaskSignedURLs(text string) string {
	if text == "" || !strings.HasPrefix(text, "http") {
		return text
	}
	parsed, err := url.Parse(strings.TrimSpace(text))
	if err != nil {
		return text
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = "redacted"
	}
	parsed.User = nil
	parsed.Fragment = ""

	return parsed.String()
}

// Content-aware redaction must succeed before any body is persisted. Malformed
// structured data is marked unavailable rather than retaining possible credentials.
func evidenceRedactBody(body []byte, contentType string) ([]byte, error) {
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
			part, err := reader.NextRawPart()
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
			header := part.Header
			for key := range header {
				if _, ok := evidenceCredentialHeaderNames[strings.ToLower(key)]; ok {
					header.Set(key, evidenceRedactedPlaceholder)
				}
			}
			if isEvidenceCredentialKey(part.FormName()) {
				payload = []byte(evidenceRedactedPlaceholder)
			} else if part.FileName() == "" && strings.Contains(part.Header.Get("Content-Type"), "json") {
				payload, err = evidenceRedactBody(payload, "application/json")
				if err != nil {
					return nil, err
				}
			}
			dest, err := writer.CreatePart(header)
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
		for key := range values {
			if isEvidenceCredentialKey(key) {
				values.Set(key, evidenceRedactedPlaceholder)
			}
		}
		return []byte(values.Encode()), nil
	}
	if mediaType == "text/event-stream" {
		lines := bytes.Split(body, []byte("\n"))
		for i, line := range lines {
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			value := bytes.TrimSpace(line[5:])
			if len(value) == 0 || string(value) == "[DONE]" {
				continue
			}
			redacted, err := evidenceRedactBody(value, "application/json")
			if err != nil {
				return nil, err
			}
			lines[i] = append([]byte("data: "), redacted...)
		}
		return bytes.Join(lines, []byte("\n")), nil
	}
	trimmed := bytes.TrimSpace(body)
	if strings.Contains(mediaType, "json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')) {
		var decoded any
		if err := common.Unmarshal(trimmed, &decoded); err != nil {
			return nil, fmt.Errorf("structured evidence is incomplete")
		}
		return common.Marshal(evidenceRedactValue(decoded))
	}
	return body, nil
}
