package relay

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestImageTransportRetryProtectionStaysScoped(t *testing.T) {
	for _, tc := range []struct {
		name    string
		apiType int
		model   string
		skip    bool
	}{
		{"gemini image", constant.APITypeGemini, "gemini-3.1-flash-image", true},
		{"vertex image", constant.APITypeVertexAi, "gemini-3.1-flash-image", true},
		{"native imagen", constant.APITypeGemini, "imagen-4.0-generate-001", false},
		{"other channel", constant.APITypeOpenAI, "gemini-3.1-flash-image", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: tc.apiType, UpstreamModelName: tc.model}}
			apiErr := imageRequestTransportError(info, errors.New("connection closed"))
			assert.Equal(t, tc.skip, types.IsSkipRetryError(apiErr))
		})
	}
}
