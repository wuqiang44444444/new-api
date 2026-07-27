package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type taskCreateMultipartFileDigest struct {
	Field       string `json:"field"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	SHA256      string `json:"sha256"`
}

type taskCreateMultipartDigest struct {
	Values map[string][]string             `json:"values"`
	Files  []taskCreateMultipartFileDigest `json:"files"`
}

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
	case strings.Contains(contentType, gin.MIMEMultipartPOSTForm):
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return "", err
		}
		defer form.RemoveAll()
		canonical := taskCreateMultipartDigest{Values: form.Value}
		for field, headers := range form.File {
			for _, header := range headers {
				fileDigest, err := hashTaskCreateMultipartFile(field, header)
				if err != nil {
					return "", err
				}
				canonical.Files = append(canonical.Files, fileDigest)
			}
		}
		sort.Slice(canonical.Files, func(i, j int) bool {
			left, right := canonical.Files[i], canonical.Files[j]
			if left.Field != right.Field {
				return left.Field < right.Field
			}
			if left.Filename != right.Filename {
				return left.Filename < right.Filename
			}
			return left.SHA256 < right.SHA256
		})
		encoded, err := common.Marshal(canonical)
		if err != nil {
			return "", err
		}
		_, _ = digest.Write(encoded)
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

func hashTaskCreateMultipartFile(field string, header *multipart.FileHeader) (taskCreateMultipartFileDigest, error) {
	file, err := header.Open()
	if err != nil {
		return taskCreateMultipartFileDigest{}, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return taskCreateMultipartFileDigest{}, err
	}
	return taskCreateMultipartFileDigest{
		Field:       field,
		Filename:    header.Filename,
		Size:        header.Size,
		ContentType: header.Header.Get("Content-Type"),
		SHA256:      hex.EncodeToString(digest.Sum(nil)),
	}, nil
}
