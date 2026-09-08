package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskErrorWrapperDoesNotExposeRawUpstreamBodiesOrURLs(t *testing.T) {
	tests := []string{
		`{"error":{"message":"provider secret"}}`,
		`body: access_token=secret`,
		`Post "https://provider.example/private": dial tcp failed`,
	}
	for _, text := range tests {
		taskError := TaskErrorWrapper(errors.New(text), "upstream_failed", 502)
		assert.NotContains(t, taskError.Message, "secret")
		assert.NotContains(t, taskError.Message, "https://")
		assert.NotContains(t, taskError.Message, "{\"error\"")
		assert.Equal(t, taskError.Message, taskError.Error.Error())
	}
}
