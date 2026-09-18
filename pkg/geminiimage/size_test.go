package geminiimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolutionContract(t *testing.T) {
	for _, tc := range []struct {
		model, size, ratio, tier string
		valid                    bool
	}{
		{"gemini-3.1-flash-lite-image", "", "", "", true},
		{"gemini-3.1-flash-lite-image", " auto ", "", "", true},
		{"gemini-3.1-flash-lite-image", "1024x1024", "1:1", "1K", true},
		{"gemini-3.1-flash-lite-image", "1280x720", "", "", false},
		{"gemini-3.1-flash-lite-image", "1699x1699", "", "", false},
		{"gemini-3.1-flash-lite-image", "1700x1700", "", "", false},
		{"gemini-3.1-flash-lite-image", "1920x1080", "", "", false},
		{"gemini-3.1-flash-lite-image", "3840x2160", "", "", false},
		{"gemini-3.1-flash-lite-image", "1024x1000", "", "", false},
		{"gemini-3.1-flash-lite-image", "1K", "", "", false},
		{"gemini-3.1-flash-lite-image", "0x0", "", "", false},
		{"gemini-3.1-flash-lite-image", "18446744073709551616x1", "", "", false},
		{"gemini-3.1-flash-image", "1920x1080", "16:9", "2K", true},
		{"gemini-3-pro-image", "3840x2160", "16:9", "4K", true},
	} {
		t.Run(tc.model+"/"+tc.size, func(t *testing.T) {
			ratio, tier, err := Resolve(tc.model, tc.size)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.ratio, ratio)
			assert.Equal(t, tc.tier, tier)
		})
	}
}
