package doubao

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
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

func TestModelArkContractPayloadInjectsOnlyHashedSubject(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(context, constant.ContextKeyEndUserSubjectHash, "opaque-subject-hash")
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:   "seedance-model",
			Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("video")}},
		},
	})

	payload, typed, err := (&TaskAdaptor{}).modelArkContractPayload(context)

	require.NoError(t, err)
	assert.True(t, typed)
	assert.Equal(t, "opaque-subject-hash", payload.SafetyIdentifier)
}

func TestModelArkRelayCapabilityRejectsUnsupportedMediaBeforeBilling(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20Oversea,
		Content: []dto.ModelArkVideoContent{{
			Type: "video_url", Role: common.GetPointer("reference_video"),
			VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"},
		}},
	}

	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Oversea)
	require.True(t, ok)
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "video content")
}

func TestModelArkRelayCapabilityRejectsEndFrameWithReferenceImageBeforeBilling(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20Oversea,
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

	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Oversea)
	require.True(t, ok)
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "cannot be combined")
}
