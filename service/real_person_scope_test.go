package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndUserSubjectHashIsOpaqueAndAppScoped(t *testing.T) {
	first, err := EndUserSubjectHash(101, "customer_subject-42")
	require.NoError(t, err)
	again, err := EndUserSubjectHash(101, "customer_subject-42")
	require.NoError(t, err)
	otherApp, err := EndUserSubjectHash(102, "customer_subject-42")
	require.NoError(t, err)

	assert.Equal(t, first, again)
	assert.NotEqual(t, first, otherApp)
	assert.NotContains(t, first, "customer_subject")
	assert.Len(t, first, 64)
}

func TestEndUserSubjectRejectsDirectIdentifiersAndAmbiguousValues(t *testing.T) {
	for _, value := range []string{
		"", "person@example.com", "has space", "slash/value", "\n", "*",
		"13800138000", "11010519491231002X", "张三", "johnsmith", "123-456-7890",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := EndUserSubjectHash(101, value)
			require.ErrorIs(t, err, ErrInvalidEndUserSubject)
		})
	}
	_, err := EndUserSubjectHash(0, "subject-1")
	require.ErrorIs(t, err, ErrInvalidEndUserSubject)
}

func TestRealPersonVerificationExpiryUsesProviderValueOrSafeFallback(t *testing.T) {
	const now int64 = 1_800_000_000
	assert.Equal(t, now+30*60, realPersonVerificationExpiresAt(0, now))
	assert.Equal(t, now+30*60, realPersonVerificationExpiresAt(now-1, now))
	assert.Equal(t, now+900, realPersonVerificationExpiresAt(now+900, now))
}
