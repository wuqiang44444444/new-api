package doubao

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelArkContractPayloadPreservesExplicitFalse(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:         "seedance-model",
			Content:       []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("video")}},
			GenerateAudio: common.GetPointer(false),
		},
	})

	payload, typed, err := (&TaskAdaptor{}).modelArkContractPayload(context)

	require.NoError(t, err)
	assert.True(t, typed)
	require.NotNil(t, payload.GenerateAudio)
	assert.False(t, bool(*payload.GenerateAudio))
}

func TestModelArkRelayCapabilityRejectsUnsupportedMediaBeforeBilling(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model: "seedance-model",
		Content: []dto.ModelArkVideoContent{{
			Type: "video_url", Role: common.GetPointer("reference_video"),
			VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"},
		}},
	}

	reason := dto.ModelArkVideoProfileIncompatibility(request, dto.VideoUpstreamProfileThirdPartyRelay, false)

	assert.Contains(t, reason, "video_url")
}

func TestModelArkRelayCapabilityRejectsEndFrameWithReferenceImageBeforeBilling(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model: "seedance-model",
		Content: []dto.ModelArkVideoContent{
			{
				Type: "image_url", Role: common.GetPointer("first_frame"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/first.png"},
			},
			{
				Type: "image_url", Role: common.GetPointer("last_frame"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/last.png"},
			},
			{
				Type: "image_url", Role: common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/reference.png"},
			},
		},
	}

	reason := dto.ModelArkVideoProfileIncompatibility(request, dto.VideoUpstreamProfileThirdPartyRelay, false)

	assert.Contains(t, reason, "cannot be combined")
	assert.Empty(t, dto.ModelArkVideoProfileIncompatibility(request, dto.VideoUpstreamProfileOfficial, false))
}
