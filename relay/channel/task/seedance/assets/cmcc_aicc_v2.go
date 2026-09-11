package assets

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const cmccAICCV2BaseURL = "https://ecloud.10086.cn/api/openapi-maas/exp/aicc/v2"
const cmccAICCMaxResponseBytes = 4 << 20

type CMCCAICCV2Adapter struct {
	baseURL   string
	accessKey string
	secretKey string
	http      HTTPDoer
	now       func() time.Time
	readNonce func([]byte) (int, error)
}

func NewCMCCAICCV2Adapter(credential string, httpClient HTTPDoer) (*CMCCAICCV2Adapter, error) {
	parts := strings.SplitN(strings.TrimSpace(credential), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("CMCC AICC credential must use ACCESS_KEY|SECRET_KEY")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CMCCAICCV2Adapter{
		baseURL: cmccAICCV2BaseURL, accessKey: strings.TrimSpace(parts[0]),
		secretKey: strings.TrimSpace(parts[1]), http: httpClient, now: time.Now, readNonce: rand.Read,
	}, nil
}

func (a *CMCCAICCV2Adapter) signedQuery(method, escapedPath string, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := a.readNonce(nonce); err != nil {
		return "", err
	}
	values := map[string]string{
		"AccessKey": a.accessKey,
		// The CMCC V2.0 endpoint validates Beijing wall-clock time with a literal Z,
		// matching the official SDK's localtime-based signing behavior.
		"Timestamp":        now.UTC().Add(8 * time.Hour).Format("2006-01-02T15:04:05Z"),
		"SignatureMethod":  "HmacSHA256",
		"SignatureVersion": "V2.0",
		"SignatureNonce":   hex.EncodeToString(nonce),
	}
	canonical := cmccCanonicalQuery(values)
	queryHash := sha256.Sum256([]byte(canonical))
	stringToSign := strings.ToUpper(method) + "\n" + cmccPercentEncode(escapedPath) + "\n" + hex.EncodeToString(queryHash[:])
	mac := hmac.New(sha256.New, []byte("BC_SIGNATURE&"+a.secretKey))
	_, _ = mac.Write([]byte(stringToSign))
	values["Signature"] = hex.EncodeToString(mac.Sum(nil))
	return cmccCanonicalQuery(values), nil
}

func cmccCanonicalQuery(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, cmccPercentEncode(key)+"="+cmccPercentEncode(values[key]))
	}
	return strings.Join(parts, "&")
}

func cmccPercentEncode(value string) string {
	const upperHex = "0123456789ABCDEF"
	var encoded strings.Builder
	for i := 0; i < len(value); i++ {
		character := value[i]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' || character == '~' {
			encoded.WriteByte(character)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(upperHex[character>>4])
		encoded.WriteByte(upperHex[character&15])
	}
	return encoded.String()
}
