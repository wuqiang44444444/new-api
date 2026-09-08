package relay

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNativeImageResponseBudgetNeverReturnsTruncatedSuccess(t *testing.T) {
	for _, tc := range []struct {
		body string
		over bool
	}{{"123", false}, {"1234", false}, {"12345", true}} {
		t.Run(tc.body, func(t *testing.T) {
			result, err := readNativeImageResponse(strings.NewReader(tc.body), 4)
			if tc.over {
				require.ErrorIs(t, err, errNativeImageResponseTooLarge)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(result))
			}
		})
	}
}
