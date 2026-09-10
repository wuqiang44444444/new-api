package azurebatch

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestBatchResultRejectsMissingOrMalformedUsage(t *testing.T) {
	for _, line := range []string{
		`{"custom_id":"a","response":{"status_code":200,"body":{}}}`,
		`{"custom_id":"a","response":{"status_code":200,"body":{"usage":{"prompt_tokens":10,"completion_tokens":-1,"total_tokens":9}}}}`,
		`{"custom_id":"a","response":{"status_code":200,"body":{"usage":{"prompt_tokens":1.5,"completion_tokens":1,"total_tokens":2.5}}}}`,
		`{"custom_id":"a"}`, `{`,
	} {
		_, err := ParseResultLines(strings.NewReader(line))
		require.Error(t, err)
	}
}
