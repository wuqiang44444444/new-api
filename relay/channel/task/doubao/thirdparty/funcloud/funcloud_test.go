package funcloud

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestInjectsFixedDefaultsAndPreservesFalseAndZero(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard)
	require.True(t, ok)
	prompt := "product shot"
	falseValue := false
	zero := 0
	request := &dto.ModelArkVideoCreateRequest{
		Model:         model.VideoSKUSeedance20Standard,
		Content:       []dto.ModelArkVideoContent{{Type: "text", Text: &prompt}},
		GenerateAudio: &falseValue,
		Seed:          &zero,
	}
	body, err := CreateRequest(request, capability)
	require.NoError(t, err)
	var output map[string]any
	require.NoError(t, common.Unmarshal(body, &output))
	assert.Equal(t, "720p", output["resolution"])
	assert.Equal(t, float64(5), output["duration"])
	assert.Equal(t, false, output["generateAudio"])
	assert.Equal(t, float64(0), output["seed"])
	assert.NotContains(t, output, "model")
}

func TestCreateResponseKeepsAllApplicationErrorsAmbiguous(t *testing.T) {
	for _, code := range []int{10002, 10005, 10006, 30003, 90003, 19999} {
		_, err := CreateResponse([]byte(fmt.Sprintf(`{"code":%d,"msg":"provider error","data":{}}`, code)))
		var createErr *CreateError
		require.ErrorAs(t, err, &createErr)
		assert.Equal(t, code, createErr.Code)
	}

	body, err := CreateResponse([]byte(`{"code":0,"msg":"success","data":{"taskId":"task_1","status":"processing"}}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_1"}`, string(body))
}

func TestTaskResponseNormalizesAndRejectsContractConflicts(t *testing.T) {
	body, err := TaskResponse([]byte(`{
		"code":0,"data":{"taskId":"task_1","status":"completed",
		"result":["https://cdn.example/video.mp4"],
		"output":{"id":"task_1","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"}}}}
	`), "task_1")
	require.NoError(t, err)
	var output map[string]any
	require.NoError(t, common.Unmarshal(body, &output))
	assert.Equal(t, "succeeded", output["status"])

	_, err = TaskResponse([]byte(`{"code":0,"data":{"taskId":"wrong","status":"processing"}}`), "task_1")
	var violation *relaycommon.UpstreamContractViolation
	assert.True(t, errors.As(err, &violation))

	_, err = TaskResponse([]byte(`{"code":0,"data":{"taskId":"task_1","status":"success","result":["http://unsafe.example/video.mp4"]}}`), "task_1")
	assert.True(t, errors.As(err, &violation))

	body, err = TaskResponse([]byte(`{"code":0,"data":{"taskId":"task_1","status":"failed","errorCode":"FAST_REJECTED","errorMsg":"rejected","output":{"id":"task_2","status":"failed"}}}`), "task_1")
	require.NoError(t, err)
	assert.Contains(t, string(body), "FAST_REJECTED")

	_, err = TaskResponse([]byte(`{"code":0,"data":{"taskId":"task_1","status":"success","result":["https://cdn.example/a.mp4","https://cdn.example/b.mp4"]}}`), "task_1")
	assert.True(t, errors.As(err, &violation))

	_, err = TaskResponse([]byte(`{"code":90003,"msg":"server internal error","data":{}}`), "task_1")
	assert.True(t, errors.As(err, &violation))
}

func TestSanitizeProviderTextPreservesUTF8WhenTruncated(t *testing.T) {
	value := sanitizeProviderText(strings.Repeat("界", 513))
	assert.True(t, utf8.ValidString(value))
	assert.Len(t, []rune(value), 512)
}

func TestCreateRequestRejectsUnresolvedAssetsAndMediaBeyondContract(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard)
	require.True(t, ok)
	prompt := "product"
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20Standard,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: &prompt},
			{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "asset://ast_01234567890123456789012345678901"}},
		},
	}
	_, err := CreateRequest(request, capability)
	require.Error(t, err)

	request.Content = []dto.ModelArkVideoContent{{Type: "text", Text: &prompt}}
	for i := 0; i < 4; i++ {
		request.Content = append(request.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example/image.png"}})
	}
	_, err = CreateRequest(request, capability)
	require.Error(t, err)
}

func TestCreateRequestRejectsMediaOutsideFunCloudContract(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard)
	require.True(t, ok)
	prompt := "product"

	tests := []struct {
		name  string
		media dto.ModelArkVideoContent
	}{
		{
			name:  "image role",
			media: dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_video"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example/image.png"}},
		},
		{
			name:  "video role",
			media: dto.ModelArkVideoContent{Type: "video_url", Role: common.GetPointer("reference_image"), VideoURL: &dto.VideoMediaURL{URL: "https://cdn.example/video.mp4"}},
		},
		{
			name:  "audio role",
			media: dto.ModelArkVideoContent{Type: "audio_url", Role: common.GetPointer("reference_image"), AudioURL: &dto.VideoMediaURL{URL: "https://cdn.example/audio.mp3"}},
		},
		{
			name:  "plain HTTP",
			media: dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "http://cdn.example/image.png"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model:   model.VideoSKUSeedance20Standard,
				Content: []dto.ModelArkVideoContent{{Type: "text", Text: &prompt}, test.media},
			}
			_, err := CreateRequest(request, capability)
			require.Error(t, err)
		})
	}
}
