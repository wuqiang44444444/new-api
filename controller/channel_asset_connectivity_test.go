package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelConnectivityPublicErrorHidesUntypedErrors(t *testing.T) {
	err := errors.New(`Post "https://provider.example/asset/query?AccessKey=secret&Signature=secret": EOF`)

	message, code := channelConnectivityPublicError(err)

	assert.Equal(t, "channel connectivity check failed", message)
	assert.Empty(t, code)
	assert.NotContains(t, message, "AccessKey")
	assert.NotContains(t, message, "Signature")
}
