package service

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AWS Signature Version 4 官方示例向量（“Example: GET Object”与
// “Example: Using the query string”）验证签名实现与 S3 兼容端点一致。
func TestSigV4SignRequestAWSVector(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=0-9")
	signingTime := time.Date(2013, time.May, 24, 0, 0, 0, 0, time.UTC)
	SigV4SignRequest(req, SigV4Credentials{
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}, "us-east-1", emptyPayloadSHA256, signingTime)

	authorization := req.Header.Get("Authorization")
	require.NotEmpty(t, authorization)
	assert.Contains(t, authorization, "Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request")
	assert.Contains(t, authorization, "SignedHeaders=host;range;x-amz-content-sha256;x-amz-date")
	assert.Contains(t, authorization, "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41")
	assert.Equal(t, "20130524T000000Z", req.Header.Get("x-amz-date"))
}

func TestSigV4PresignURLAWSVector(t *testing.T) {
	url, err := SigV4PresignURL(
		http.MethodGet,
		"https://examplebucket.s3.amazonaws.com/test.txt",
		SigV4Credentials{
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		"us-east-1",
		86400*time.Second,
		time.Date(2013, time.May, 24, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.Contains(t, url, "https://examplebucket.s3.amazonaws.com/test.txt?")
	assert.Contains(t, url, "X-Amz-Algorithm=AWS4-HMAC-SHA256")
	assert.Contains(t, url, "X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request")
	assert.Contains(t, url, "X-Amz-Date=20130524T000000Z")
	assert.Contains(t, url, "X-Amz-Expires=86400")
	assert.Contains(t, url, "X-Amz-SignedHeaders=host")
	assert.Contains(t, url, "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404")
}

func TestSigV4PresignURLRequiresPositiveTTL(t *testing.T) {
	_, err := SigV4PresignURL(http.MethodGet, "https://s3.example.com/bucket/key",
		SigV4Credentials{AccessKey: "a", SecretKey: "b"}, "us-east-1", 0, time.Now())
	assert.Error(t, err)
}

func TestSigV4PresignURLQueryOrderingStable(t *testing.T) {
	credentials := SigV4Credentials{AccessKey: "ak", SecretKey: "sk"}
	first, err := SigV4PresignURL(http.MethodGet, "https://s3.example.com/b/key",
		credentials, "us-east-1", 300*time.Second, time.Unix(1700000000, 0))
	require.NoError(t, err)
	second, err := SigV4PresignURL(http.MethodGet, "https://s3.example.com/b/key",
		credentials, "us-east-1", 300*time.Second, time.Unix(1700000000, 0))
	require.NoError(t, err)
	assert.Equal(t, first, second)
	// 签名不进入 Authorization 头形态；完整签名 URL 不得包含空格。
	assert.False(t, strings.Contains(first, " "))
}
