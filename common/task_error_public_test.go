package common

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPublicTaskErrorPreservesBusinessDetails(t *testing.T) {
	for _, message := range []string{
		"The request failed because the output video may be related to copyright restrictions.",
		"输入内容可能包含敏感信息，请检查后重试。",
		"asset does not exist", "duration must be between 4 and 15 seconds",
		"Invalid request body: duration must be between 4 and 15 seconds",
		"request body: 输入内容可能包含敏感信息，请检查后重试。",
		"İbody: 参数错误", "Kbody: invalid duration", "max_token: 4096 is too large",
	} {
		assert.Equal(t, message, PublicTaskErrorMessage(message))
	}
	message := "FunCloud: image URL expired https://private.invalid/image?signature=secret; channel_id: 72; api_key=secret"
	safe := PublicTaskErrorMessage(message)
	assert.Contains(t, safe, "image URL expired")
	for _, secret := range []string{"FunCloud", "72", "private.invalid", "secret", "api_key"} {
		assert.NotContains(t, safe, secret)
	}
	assert.Equal(t, safe, PublicTaskErrorMessage(safe), "read-time protection must not destroy already normalized messages")
	assert.Equal(t, "Task status could not be retrieved. Please contact support with the request ID.", PublicTaskErrorMessage(`poll failed: unrecognized; body={"account":"secret"}`))
	assert.Equal(t, "invalid duration", PublicTaskErrorMessage(`invalid duration; body={"account":"secret"}`))
	assert.Equal(t, "invalid duration [redacted]", PublicTaskErrorMessage("invalid duration api_key：secret"))
	assert.Equal(t, "decode response body: unexpected end of JSON input", SanitizeTaskDiagnostic("decode response body: unexpected end of JSON input"))
	assert.NotContains(t, PublicTaskErrorMessage("获取渠道信息失败，请联系管理员，渠道ID：72"), "72")
	long := PublicTaskErrorMessage(strings.Repeat("猫", 400))
	assert.True(t, utf8.ValidString(long))
	assert.LessOrEqual(t, len(long), 515)
	assert.NotContains(t, long, "\uFFFD")
}

func TestTaskErrorDoesNotExposeAuthenticationHeaders(t *testing.T) {
	for _, message := range []string{
		"request rejected; Authorization: Basic Zml4dHVyZTpwYXNzd29yZA==",
		"request rejected; Cookie: first=fixture; session=fixture-session",
		`request rejected; "Authorization": "Bearer fixture-token"`,
	} {
		assert.Equal(t, "Video service request failed", PublicTaskErrorMessage(message))
		assert.Equal(t, "Video service request failed", SanitizeTaskDiagnostic(message))
	}
}
