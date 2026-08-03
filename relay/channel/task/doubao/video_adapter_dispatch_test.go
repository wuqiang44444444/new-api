package doubao

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoAdapterVersionFromFetchBodyDefaultsOmniTaskToV2(t *testing.T) {
	version, err := videoAdapterVersionFromFetchBody(
		map[string]any{},
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
	)
	require.NoError(t, err)
	assert.True(t, version.IsJSONVideoOmniV2())
}

func TestVideoAdapterVersionFromFetchBodyRejectsMismatchedVersion(t *testing.T) {
	for _, frozen := range []string{
		"54:third_party_json_video_omni_reference:v1",
		"54:official:v2",
	} {
		_, err := videoAdapterVersionFromFetchBody(
			map[string]any{videoUpstreamAdapterVersionKey: frozen},
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		)
		require.Error(t, err)
		var violation *relaycommon.UpstreamContractViolation
		assert.True(t, errors.As(err, &violation))
	}
}
