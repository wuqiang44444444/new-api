package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicTaskErrorCodeRejectsCredentialDiagnostics(t *testing.T) {
	for _, code := range []string{
		"access_token:fixture-secret", "api_key:fixture-secret", "client_secret:fixture-secret",
		"Authorization:fixture-secret", "Cookie:fixture-secret", "sk-fixture-secret",
	} {
		t.Run(code, func(t *testing.T) { assert.Empty(t, PublicTaskErrorCode(code)) })
	}
	for _, code := range []string{"AuditSubmitIllegal", "ProviderBusy", "TokenLimitExceeded", "Error.Code:123", "generation_failed"} {
		assert.Equal(t, code, PublicTaskErrorCode(code))
	}
}
