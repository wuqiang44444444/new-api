package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 代码内 AWS Signature V4 实现（S3 兼容端点通用，含 Azure Blob S3 网关、
// MinIO、R2、OSS）。不引入新的 Go 依赖；签名输出与官方 SDK 一致，可用
// AWS 公开测试向量验证（sigv4_test.go）。

const (
	sigV4Algorithm     = "AWS4-HMAC-SHA256"
	sigV4Service       = "s3"
	sigV4Request       = "aws4_request"
	sigV4TimeFormat    = "20060102T150405Z"
	sigV4DateFormat    = "20060102"
	unsignedPayload    = "UNSIGNED-PAYLOAD"
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// SigV4Credentials 是从部署配置读取的静态 S3 凭据。
type SigV4Credentials struct {
	AccessKey string
	SecretKey string
}

// SigV4SignRequest applies header-style SigV4 authentication in place.
// payloadHash is the hex SHA-256 of the body, or sigV4UnsignedPayload.
func SigV4SignRequest(req *http.Request, credentials SigV4Credentials, region, payloadHash string, signingTime time.Time) {
	amzDate := signingTime.UTC().Format(sigV4TimeFormat)
	dateStamp := signingTime.UTC().Format(sigV4DateFormat)
	if payloadHash == "" {
		payloadHash = emptyPayloadSHA256
	}

	req.Header.Set("x-amz-date", amzDate)
	if req.Header.Get("host") == "" && req.Host != "" {
		req.Header.Set("host", req.Host)
	}
	req.Header.Del("x-amz-content-sha256")
	req.Header.Set("x-amz-content-sha256", payloadHash)

	signedHeaders, canonicalHeaders := canonicalSigV4Headers(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalSigV4URI(req.URL),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, region, sigV4Service, sigV4Request}, "/")
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSigV4Signature(credentials.SecretKey, dateStamp, region, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigV4Algorithm, credentials.AccessKey, scope, signedHeaders, signature,
	))
}

// SigV4PresignURL produces a query-string presigned URL valid for ttl
// seconds. method must be GET or HEAD for downloads, PUT for uploads.
func SigV4PresignURL(method, rawURL string, credentials SigV4Credentials, region string, ttl time.Duration, signingTime time.Time) (string, error) {
	if method == "" {
		method = http.MethodGet
	}
	if ttl <= 0 {
		return "", fmt.Errorf("presign ttl must be positive")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("object URL must be absolute")
	}

	amzDate := signingTime.UTC().Format(sigV4TimeFormat)
	dateStamp := signingTime.UTC().Format(sigV4DateFormat)
	scope := strings.Join([]string{dateStamp, region, sigV4Service, sigV4Request}, "/")

	query := parsed.Query()
	query.Set("X-Amz-Algorithm", sigV4Algorithm)
	query.Set("X-Amz-Credential", credentials.AccessKey+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")
	parsed.RawQuery = encodeSigV4Query(query)

	// host 是唯一被签名的 header（query 预签名不携带 Authorization）。
	canonicalRequest := strings.Join([]string{
		method,
		canonicalSigV4URI(parsed),
		parsed.RawQuery,
		"host:" + strings.ToLower(parsed.Host) + "\n",
		"host",
		unsignedPayload,
	}, "\n")

	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSigV4Signature(credentials.SecretKey, dateStamp, region, stringToSign))

	separator := "?"
	if parsed.RawQuery != "" {
		separator = "&"
	}
	return parsed.String() + separator + "X-Amz-Signature=" + signature, nil
}

func canonicalSigV4URI(parsed *url.URL) string {
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return path
}

func canonicalSigV4Headers(req *http.Request) (signedHeaders, canonical string) {
	names := make([]string, 0, len(req.Header)+1)
	values := make(map[string]string, len(req.Header)+1)
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	names = append(names, "host")
	values["host"] = strings.ToLower(host)
	for name, headerValues := range req.Header {
		lower := strings.ToLower(strings.TrimSpace(name))
		if !isSignedSigV4Header(lower) {
			continue
		}
		trimmed := make([]string, 0, len(headerValues))
		for _, value := range headerValues {
			trimmed = append(trimmed, compressSigV4Whitespace(value))
		}
		if _, exists := values[lower]; !exists {
			names = append(names, lower)
		}
		values[lower] = strings.Join(trimmed, ",")
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(name)
		builder.WriteString(":")
		builder.WriteString(values[name])
		builder.WriteString("\n")
	}
	return strings.Join(names, ";"), builder.String()
}

func isSignedSigV4Header(name string) bool {
	switch name {
	case "authorization", "user-agent":
		return false
	default:
		return strings.HasPrefix(name, "x-amz-") || name == "content-type" || name == "content-md5" || name == "range"
	}
}

func compressSigV4Whitespace(value string) string {
	if !strings.Contains(value, "  ") && !strings.Contains(value, "\t") {
		return strings.TrimSpace(value)
	}
	fields := strings.Fields(value)
	return strings.Join(fields, " ")
}

// encodeSigV4Query 按 RFC 3986 重新编码 query，键排序满足规范形式要求。
func encodeSigV4Query(query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for i, key := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		builder.WriteString(url.QueryEscape(key))
		builder.WriteString("=")
		builder.WriteString(url.QueryEscape(query.Get(key)))
	}
	return builder.String()
}

func hmacSigV4Signature(secretKey, dateStamp, region, stringToSign string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, sigV4Service)
	signingKey := hmacSHA256(serviceKey, sigV4Request)
	return hmacSHA256(signingKey, stringToSign)
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
