package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func taskCreateRequestHash(c *gin.Context, protocol string) (string, error) {
	digest := sha256.New()
	_, _ = io.WriteString(digest, c.Request.Method+"\n"+c.Request.URL.Path+"\n"+protocol+"\n")

	contentType := c.GetHeader("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return "", err
		}
		body, err := storage.Bytes()
		if err != nil {
			return "", err
		}
		canonical, err := canonicalTaskCreateJSON(body)
		if err != nil {
			return "", err
		}
		_, _ = digest.Write(canonical)
	default:
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return "", err
		}
		if _, err := storage.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		if _, err := io.Copy(digest, storage); err != nil {
			return "", err
		}
		if _, err := storage.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		c.Request.Body = io.NopCloser(common.ReaderOnly(storage))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func canonicalTaskCreateJSON(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	switch common.GetJsonType(json.RawMessage(trimmed)) {
	case "object":
		var object map[string]json.RawMessage
		if err := common.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var result bytes.Buffer
		result.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				result.WriteByte(',')
			}
			encodedKey, err := common.Marshal(key)
			if err != nil {
				return nil, err
			}
			value, err := canonicalTaskCreateJSON(object[key])
			if err != nil {
				return nil, err
			}
			result.Write(encodedKey)
			result.WriteByte(':')
			result.Write(value)
		}
		result.WriteByte('}')
		return result.Bytes(), nil
	case "array":
		var values []json.RawMessage
		if err := common.Unmarshal(trimmed, &values); err != nil {
			return nil, err
		}
		var result bytes.Buffer
		result.WriteByte('[')
		for index, raw := range values {
			if index > 0 {
				result.WriteByte(',')
			}
			value, err := canonicalTaskCreateJSON(raw)
			if err != nil {
				return nil, err
			}
			result.Write(value)
		}
		result.WriteByte(']')
		return result.Bytes(), nil
	case "string":
		var value string
		if err := common.Unmarshal(trimmed, &value); err != nil {
			return nil, err
		}
		return common.Marshal(value)
	case "number":
		var value json.Number
		if err := common.Unmarshal(trimmed, &value); err != nil {
			return nil, err
		}
		return common.Marshal(value)
	case "boolean", "null":
		var value any
		if err := common.Unmarshal(trimmed, &value); err != nil {
			return nil, err
		}
		return common.Marshal(value)
	default:
		return nil, io.ErrUnexpectedEOF
	}
}
