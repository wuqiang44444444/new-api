// Package assets implements Seedance asset-library protocols.
package assets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	officialActionService       = "ark"
	officialActionVersion       = "2024-01-01"
	volcengineActionBaseURL     = "https://ark.cn-beijing.volcengineapi.com"
	volcengineActionRegion      = "cn-beijing"
	bytePlusActionBaseURLFormat = "https://ark.%s.byteplusapi.com"
)

type OfficialActionAdapter struct {
	baseURL         string
	accessKey       string
	secretKey       string
	region          string
	providerProject string
	http            HTTPDoer
	now             func() time.Time
}

func NewVolcengineActionAdapter(credential, providerProject string, httpClient HTTPDoer) (*OfficialActionAdapter, error) {
	return newOfficialActionAdapter(volcengineActionBaseURL, credential, volcengineActionRegion, providerProject, httpClient)
}

func NewBytePlusActionAdapter(credential, region, providerProject string, httpClient HTTPDoer) (*OfficialActionAdapter, error) {
	region = strings.TrimSpace(region)
	return newOfficialActionAdapter(fmt.Sprintf(bytePlusActionBaseURLFormat, region), credential, region, providerProject, httpClient)
}

func newOfficialActionAdapter(baseURL, credential, region, providerProject string, httpClient HTTPDoer) (*OfficialActionAdapter, error) {
	parts := strings.SplitN(strings.TrimSpace(credential), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("official Action credential must use ACCESS_KEY|SECRET_KEY")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("official Action endpoint is invalid")
	}
	if strings.TrimSpace(region) == "" || strings.TrimSpace(providerProject) == "" {
		return nil, fmt.Errorf("official Action Region and ProviderProject are required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OfficialActionAdapter{
		baseURL:         strings.TrimRight(parsed.String(), "/"),
		accessKey:       strings.TrimSpace(parts[0]),
		secretKey:       strings.TrimSpace(parts[1]),
		region:          strings.TrimSpace(region),
		providerProject: strings.TrimSpace(providerProject),
		http:            httpClient,
		now:             time.Now,
	}, nil
}

func (a *OfficialActionAdapter) sign(req *http.Request, payload []byte, now time.Time) {
	payloadHash := sha256Hex(payload)
	date := now.Format("20060102")
	xDate := now.Format("20060102T150405Z")
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", payloadHash)
	signedHeaders := "content-type;host;x-content-sha256;x-date"
	canonicalHeaders := "content-type:" + req.Header.Get("Content-Type") + "\n" +
		"host:" + req.URL.Host + "\n" +
		"x-content-sha256:" + payloadHash + "\n" +
		"x-date:" + xDate + "\n"
	canonicalPath := req.URL.EscapedPath()
	if canonicalPath == "" {
		canonicalPath = "/"
	}
	canonicalRequest := req.Method + "\n" +
		canonicalPath + "\n" +
		req.URL.Query().Encode() + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		payloadHash
	scope := date + "/" + a.region + "/" + officialActionService + "/request"
	stringToSign := "HMAC-SHA256\n" + xDate + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))
	dateKey := hmacSHA256([]byte(a.secretKey), date)
	regionKey := hmacSHA256(dateKey, a.region)
	serviceKey := hmacSHA256(regionKey, officialActionService)
	signingKey := hmacSHA256(serviceKey, "request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", "HMAC-SHA256 Credential="+a.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
