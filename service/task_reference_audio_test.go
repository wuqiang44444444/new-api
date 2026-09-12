package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceAudioPreparationAndPrivateSnapshot(t *testing.T) {
	// Transport does not parse duration or decide whether these audio bytes are playable.
	data := []byte("audio bytes without duration metadata")
	source := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(data)
	store := newFakeHostedImageStore()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(withHostedImageSession(context.Background(), store))
	req := &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{
		{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "https://source.example/image.png"}},
		{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: source}},
	}}
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
	SetVideoReferenceAudioInputs(c, map[int]VideoReferenceAudioInput{1: {Source: source, Data: data, MimeType: "audio/wav"}})
	info := &relaycommon.RelayInfo{UserId: 7, TaskRelayInfo: &relaycommon.TaskRelayInfo{AppID: 9}}
	require.Nil(t, PrepareVideoReferenceAudio(c, info))
	require.Nil(t, PrepareVideoReferenceAudio(c, info))
	require.Len(t, store.puts, 1)
	task := &model.Task{}
	StageVideoReferenceAudioSnapshot(c, task)
	require.Len(t, task.PrivateData.ReferenceAudio, 1)
	fact := task.PrivateData.ReferenceAudio[0]
	assert.Equal(t, 1, fact.ContentIndex)
	assert.Contains(t, fact.ObjectKey, "media/seedance/7/9/audio-")
	assert.Equal(t, data, store.puts[fact.ObjectKey])
	assert.Equal(t, "audio/wav", store.mimeTypes[fact.ObjectKey])
	assert.Equal(t, store.location, fact.StorageLocation)
	assert.Equal(t, "https://store.example/signed/"+fact.ObjectKey, req.Content[1].AudioURL.URL)

	raw, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), source)
	assert.NotContains(t, string(raw), "/signed/")

}

func TestReferenceAudioURLDoesNotNeedStorage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	source := "https://source.example/a.mp3?one=1&two=2"
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: source}}}}})
	require.Nil(t, PrepareVideoReferenceAudio(c, nil))
	task := &model.Task{}
	StageVideoReferenceAudioSnapshot(c, task)
	assert.Empty(t, task.PrivateData.ReferenceAudio)
}

func TestReferenceAudioUploadFailureHasNoSnapshot(t *testing.T) {
	store := newFakeHostedImageStore()
	store.failPut = true
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(withHostedImageSession(context.Background(), store))
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: "data:audio/wav;base64,YXVkaW8="}}}}})
	SetVideoReferenceAudioInputs(c, map[int]VideoReferenceAudioInput{0: {Source: "data:audio/wav;base64,YXVkaW8=", Data: []byte("audio"), MimeType: "audio/wav"}})
	err := PrepareVideoReferenceAudio(c, &relaycommon.RelayInfo{UserId: 7})
	require.NotNil(t, err)
	assert.Equal(t, "reference_audio_unavailable", err.Code)
	task := &model.Task{}
	StageVideoReferenceAudioSnapshot(c, task)
	assert.Empty(t, task.PrivateData.ReferenceAudio)
}

func TestMultipartAudioRequestEvidenceRedactsUntypedJSON(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("request", `{"token":"fixture-sensitive","content":[{"type":"audio_url","audio_url":{"url":"file://audio"}}]}`))
	part, err := w.CreateFormFile("audio", "sample.wav")
	require.NoError(t, err)
	_, err = part.Write([]byte("file bytes"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	redacted, err := evidenceRedactBody(body.Bytes(), w.FormDataContentType())
	require.NoError(t, err)
	assert.NotContains(t, string(redacted), "fixture-sensitive")
	reader := multipart.NewReader(bytes.NewReader(redacted), w.Boundary())
	_, err = reader.NextPart()
	require.NoError(t, err)
	audio, err := reader.NextPart()
	require.NoError(t, err)
	data, err := io.ReadAll(audio)
	require.NoError(t, err)
	assert.Equal(t, []byte("file bytes"), data)
}

func TestRepeatedReferenceAudioKeepsEachContentObject(t *testing.T) {
	store := newFakeHostedImageStore()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(withHostedImageSession(context.Background(), store))
	source := "file://audio"
	req := &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{
		{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: source}},
		{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: source}},
	}}
	input := VideoReferenceAudioInput{Source: source, Data: []byte("same audio"), MimeType: "application/octet-stream"}
	SetVideoReferenceAudioInputs(c, map[int]VideoReferenceAudioInput{0: input, 1: input})
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
	require.Nil(t, PrepareVideoReferenceAudio(c, &relaycommon.RelayInfo{UserId: 7}))
	task := &model.Task{}
	StageVideoReferenceAudioSnapshot(c, task)
	require.Len(t, task.PrivateData.ReferenceAudio, 2)
	require.Len(t, store.puts, 2)
	for i, fact := range task.PrivateData.ReferenceAudio {
		assert.Equal(t, i, fact.ContentIndex)
		assert.Equal(t, input.Data, store.puts[fact.ObjectKey])
		assert.Equal(t, "https://store.example/signed/"+fact.ObjectKey, req.Content[i].AudioURL.URL)
	}
	assert.NotEqual(t, req.Content[0].AudioURL.URL, req.Content[1].AudioURL.URL)
}

func TestReferenceAudioRequiresIngressBinding(t *testing.T) {
	for _, changed := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		store := newFakeHostedImageStore()
		c.Request = httptest.NewRequest("POST", "/", nil).WithContext(withHostedImageSession(context.Background(), store))
		source := "file://audio"
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{{Type: "audio_url", AudioURL: &dto.VideoMediaURL{URL: source}}}}})
		if changed {
			SetVideoReferenceAudioInputs(c, map[int]VideoReferenceAudioInput{0: {Source: "file://other", Data: []byte("audio"), MimeType: "audio/wav"}})
		}
		err := PrepareVideoReferenceAudio(c, &relaycommon.RelayInfo{UserId: 7})
		require.NotNil(t, err)
		assert.Equal(t, "invalid_video_parameter", err.Code)
		assert.Empty(t, store.puts)
	}
}
