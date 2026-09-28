package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestViduUnresolvedAssetsNeverReachProvider(t *testing.T) {
	for _, body := range []string{
		`{"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://fhas_missing-facts"}}]}`,
		`{"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://provider-id"}}]}`,
	} {
		out, err := resolvePluginHostedContent(nil, []byte(body), dto.VideoUpstreamProtocolViduModelArkV3)
		require.Error(t, err)
		require.Nil(t, out)
	}
}
