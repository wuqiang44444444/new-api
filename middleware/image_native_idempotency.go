package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// Native multipart order and complete file contents are semantic. Encode
// structured fields so embedded newlines cannot alias a different request.
func nativeImageMultipartHash(c *gin.Context) (string, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return "", err
	}
	defer form.RemoveAll()
	type fileFact struct {
		Name   string
		Header map[string][]string
		Size   int64
		Digest string
	}
	facts := struct {
		Path   string
		Values map[string][]string
		Files  map[string][]fileFact
	}{Path: c.Request.URL.Path, Values: form.Value, Files: make(map[string][]fileFact)}
	for key, files := range form.File {
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				return "", err
			}
			hash := sha256.New()
			size, err := io.Copy(hash, file)
			file.Close()
			if err != nil {
				return "", err
			}
			facts.Files[key] = append(facts.Files[key], fileFact{Name: header.Filename, Header: header.Header, Size: size, Digest: hex.EncodeToString(hash.Sum(nil))})
		}
	}
	encoded, err := common.Marshal(facts)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
