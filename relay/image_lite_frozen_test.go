package relay

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLiteFrozenTaskSurvivesRegistrationRemoval(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { model.DB = previousDB; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.ImageTaskSlot{}))
	var uploaded []byte
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			uploaded, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		} else {
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(store.Close)
	config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "fixture", Region: "us-east-1", Credential: "fixture", Revision: "lite-frozen"})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(config))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	var original bytes.Buffer
	require.NoError(t, png.Encode(&original, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))))
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/v1beta/models/gemini-3.1-flash-lite-image:generateContent", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(original.Bytes())+`"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1130,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}}`)
	}))
	t.Cleanup(provider.Close)
	service.InitHttpClient()
	task := &model.Task{TaskID: model.GenerateTaskID(), UserId: 1, AppID: 7, Status: model.TaskStatusInProgress, ClientProtocol: model.TaskClientProtocolImageOpenAIV1, PrivateData: model.TaskPrivateData{ImageTask: &model.TaskImageExecutionData{Operation: "generations", ChannelType: 24, ChannelBaseUrl: provider.URL, ChannelKey: "fixture", UpstreamModel: "gemini-3.1-flash-lite-image", ResponseFormat: "url", N: 1, FundsHeld: true, Parameters: &dto.ImageRequest{Prompt: "cup", Size: "1024x1024"}}}}
	info := buildFrozenImageRelayInfo(task, task.PrivateData.ImageTask)
	task.PrivateData.ImageTask.HeadersCiphertext, err = freezeImageTaskHeaders(task.TaskID, nil, info)
	require.NoError(t, err)
	require.NoError(t, db.Create(task).Error)
	settings := model_setting.GetGeminiSettings()
	previous := settings.SupportedImagineModels
	t.Cleanup(func() { settings.SupportedImagineModels = previous })
	settings.SupportedImagineModels = []string{"gemini-3.1-flash-image"}
	_, err = (&gemini.Adaptor{}).ConvertImageRequest(nil, info, *task.PrivateData.ImageTask.Parameters)
	require.Error(t, err, "new requests must obey current registration")
	outcome := ExecuteImageTask(t.Context(), task)
	require.Equal(t, service.ImageTaskOutcomeSuccess, outcome.Outcome, outcome.FailureCode)
	assert.Equal(t, 1, calls, "accepted task uses one frozen provider request")
	require.Len(t, outcome.Images, 1)
	assert.Equal(t, original.Bytes(), uploaded)
	assert.Equal(t, 1120, outcome.Usage.CompletionTokenDetails.ImageTokens)
}
