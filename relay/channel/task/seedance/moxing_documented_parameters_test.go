package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxingMappedResolutionValidationUsesDocumentedEnums(t *testing.T) {
	for _, tc := range []struct {
		model, resolution string
		accepted          bool
	}{
		{"doubao-seedance-2-0-260128-0818", "480p", true},
		{"doubao-seedance-2-0-260128-0818", "720p", true},
		{"doubao-seedance-2-0-260128-0818", "1080p", true},
		{"doubao-seedance-2-0-260128-0818", "4k", true},
		{"doubao-seedance-2-0-260128-0818", "8k", false},
		{"doubao-seedance-2-5-260628", "480p", true},
		{"doubao-seedance-2-5-260628", "720p", true},
		{"doubao-seedance-2-5-260628", "1080p", false},
		{"doubao-seedance-2-5-260628", "4k", false},
		// These pages give suggestions, not an exhaustive supported-value list.
		{"doubao-seedance-2-0-mini-260615", "1080p", true},
		{"doubao-seedance-2-0-fast-260128", "1080p", true},
	} {
		t.Run(tc.model+"/"+tc.resolution, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{Model: "customer-video", Content: contentItems(textItem("generate")), Resolution: common.GetPointer(tc.resolution)}
			context := moxingTestContext(t, request)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model, IsModelMapped: true}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			validationErr := moxingAdaptor().ValidateMappedRequest(context, info)
			_, probeErr := moxingAdaptor().BuildTaskBillingProbe(context, info)
			if !tc.accepted {
				require.NotNil(t, validationErr)
				require.Error(t, probeErr)
				reader, err := moxingAdaptor().BuildRequestBody(context, info)
				require.Error(t, err)
				assert.Nil(t, reader)
				return
			}
			require.Nil(t, validationErr)
			require.NoError(t, probeErr)
			body := buildMoxingOutboundBody(t, tc.model, request)
			assert.Equal(t, tc.resolution, body["resolution"])
			assert.Equal(t, "customer-video", request.Model)
		})
	}
}

func TestMoxingOutputFormatOnlyPublishedFor25(t *testing.T) {
	for _, model := range []string{"doubao-seedance-2-0-260128-0818", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"} {
		for _, format := range []string{"mp4", "mov"} {
			t.Run(model+"/"+format, func(t *testing.T) {
				request := &dto.ModelArkVideoCreateRequest{Model: "customer", Content: contentItems(textItem("generate")), OutputFormat: common.GetPointer(format)}
				context := moxingTestContext(t, request)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model, IsModelMapped: true}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				if model == "doubao-seedance-2-5-260628" {
					require.Nil(t, moxingAdaptor().ValidateMappedRequest(context, info))
					assert.Equal(t, format, buildMoxingOutboundBody(t, model, request)["output_format"])
					return
				}
				require.NotNil(t, moxingAdaptor().ValidateMappedRequest(context, info))
				reader, err := moxingAdaptor().BuildRequestBody(context, info)
				require.Error(t, err)
				assert.Nil(t, reader)
				assert.Equal(t, format, *request.OutputFormat)
			})
		}
	}
}
