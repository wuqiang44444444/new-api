package funcloud

import (
	"errors"
	"testing"

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

func TestCreateResponseClassifiesOnlyKnownRejections(t *testing.T) {
	_, err := CreateResponse([]byte(`{"code":10002,"msg":"bad","data":{}}`))
	assert.True(t, IsTerminalCreateRejection(err))

	_, err = CreateResponse([]byte(`{"code":19999,"msg":"unknown","data":{}}`))
	assert.False(t, IsTerminalCreateRejection(err))

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
}

func TestCreateRequestRejectsUnresolvedAssetsAndMediaBeyondContract(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard)
	require.True(t, ok)
	prompt := "product"
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20Standard,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: &prompt},
			{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "asset://ast_01234567890123456789012345678901"}},
		},
	}
	_, err := CreateRequest(request, capability)
	require.Error(t, err)

	request.Content = []dto.ModelArkVideoContent{{Type: "text", Text: &prompt}}
	for i := 0; i < 4; i++ {
		request.Content = append(request.Content, dto.ModelArkVideoContent{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example/image.png"}})
	}
	_, err = CreateRequest(request, capability)
	require.Error(t, err)
}
