package controller

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceAudioCreateUploadBeforeHoldAndFreezeFacts(t *testing.T) {
	for _, outcome := range []string{"https", "base64", "file", "raw_base64", "raw_file", "repeated", "upload_failure", "sign_failure", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			testReferenceAudioCreation(t, dto.VideoUpstreamProtocolFunCloudModelArkV3, outcome)
		})
	}
}

func TestSynlinkReferenceAudioCreateInputForms(t *testing.T) {
	for _, outcome := range []string{"https", "base64", "file", "raw_base64", "raw_file", "repeated", "upload_failure", "sign_failure", "unknown"} {
		t.Run(outcome, func(t *testing.T) { testReferenceAudioCreation(t, dto.VideoUpstreamProtocolSynlinkVideoV1, outcome) })
	}
}

func testReferenceAudioCreation(t *testing.T, protocol dto.VideoUpstreamProtocol, outcome string) {
	t.Helper()
	service.InitHttpClient()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	events := []string{}
	db := setupTaskSubmissionDatabase(t, true, &events)
	seedPublishedSeedanceControllerArtifact(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.TaskCreateAttempt{}, &model.TaskCreateIdempotency{}, &model.UserSubscription{}))
	oldLog, oldCache, oldRedis, oldBatch, oldConsume := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLog
		common.MemoryCacheEnabled = oldCache
		common.RedisEnabled = oldRedis
		common.BatchUpdateEnabled = oldBatch
		common.LogConsumeEnabled = oldConsume
	})
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"customer-audio":"tiered_expr"}`, "billing_setting.billing_expr": `{"customer-audio":"tier(\"fixed\", 0.0014)"}`}))
	user := model.User{Id: 8191, Username: "audio-integration", Quota: 10000, Status: common.UserStatusEnabled, Group: "default"}
	user.SetSetting(kitdto.UserSetting{BillingPreference: "wallet_only"})
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{Id: 8191, UserId: user.Id, Key: strings.Repeat("a", 32), Status: common.TokenStatusEnabled, RemainQuota: 10000, ExpiredTime: -1, Group: "default"}
	require.NoError(t, db.Create(&token).Error)
	audio := []byte("RIFF\x04\x00\x00\x00WAVE") // No duration validation: the provider owns media validation.
	if outcome == "raw_base64" || outcome == "raw_file" {
		audio = []byte{0xff, 0xfb, 0x90, 0x64, 0, 0, 0, 0}
	}
	uploads, providerCalls := 0, 0
	store := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads++
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Contains(t, r.URL.Path, "/media/seedance/8191/8191/audio-")
		if outcome == "raw_base64" || outcome == "raw_file" {
			assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
		} else {
			assert.Contains(t, r.Header.Get("Content-Type"), "audio/")
		}
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, audio, data)
		var attempts int64
		assert.NoError(t, db.Model(&model.TaskCreateAttempt{}).Count(&attempts).Error)
		assert.Zero(t, attempts)
		var owner model.User
		assert.NoError(t, db.First(&owner, user.Id).Error)
		assert.Equal(t, 10000, owner.Quota)
		if outcome == "upload_failure" {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(200)
	}))
	if outcome == "sign_failure" {
		store.Start()
	} else {
		store.StartTLS()
	}
	t.Cleanup(store.Close)
	previousTransport := http.DefaultTransport
	if store.Client().Transport != nil {
		http.DefaultTransport = store.Client().Transport
	}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	storageConfig := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "media", Prefix: "test", AccountName: "fixture", Region: "us-east-1", Credential: "fixture-secret", Revision: outcome}
	encoded, err := common.Marshal(storageConfig)
	require.NoError(t, err)
	if outcome == "https" {
		model.NotifyObjectStorageSettingUpdate("")
	} else {
		model.NotifyObjectStorageSettingUpdate(string(encoded))
	}
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	var sendingFacts []model.TaskReferenceAudioFact
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		var attempt model.TaskCreateAttempt
		require.NoError(t, db.First(&attempt).Error)
		assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
		assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
		var snapshot struct {
			PrivateData model.TaskPrivateData `json:"private_data"`
		}
		require.NoError(t, common.Unmarshal(attempt.RecoverySnapshot, &snapshot))
		sendingFacts = snapshot.PrivateData.ReferenceAudio
		assert.NotContains(t, string(attempt.RecoverySnapshot), "base64,")
		assert.NotContains(t, string(attempt.RecoverySnapshot), "X-Amz-")
		var wire struct {
			Content        []dto.ModelArkVideoContent `json:"content"`
			RealPersonMode bool                       `json:"real_person_mode"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &wire))
		expectedAudios := 1
		if outcome == "repeated" {
			expectedAudios = 2
		}
		require.Len(t, wire.Content, 6+expectedAudios)
		assert.Equal(t, protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3, wire.RealPersonMode)
		for i := 1; i <= 5; i++ {
			assert.Equal(t, "image_url", wire.Content[i].Type)
			assert.Equal(t, "https://source.example/image.png", wire.Content[i].ImageURL.URL)
		}
		ref := wire.Content[6]
		assert.Equal(t, "audio_url", ref.Type)
		assert.Nil(t, ref.ImageURL)
		require.NotNil(t, ref.AudioURL)
		assert.Equal(t, "reference_audio", *ref.Role)
		if outcome == "https" {
			assert.Equal(t, "https://source.example/audio.wav?one=1&two=2", ref.AudioURL.URL)
			assert.Empty(t, sendingFacts)
		} else {
			require.Len(t, sendingFacts, expectedAudios)
			for i, fact := range sendingFacts {
				assert.Equal(t, 6+i, fact.ContentIndex)
				assert.Contains(t, wire.Content[6+i].AudioURL.URL, "/media/test/"+fact.ObjectKey+"?")
			}
			if outcome == "repeated" {
				assert.NotEqual(t, wire.Content[6].AudioURL.URL, wire.Content[7].AudioURL.URL)
			}
			assert.Equal(t, 6, sendingFacts[0].ContentIndex)
			assert.True(t, strings.HasPrefix(ref.AudioURL.URL, store.URL+"/media/test/"))
			assert.Contains(t, ref.AudioURL.URL, "X-Amz-Expires=86400")
		}
		w.Header().Set("Content-Type", "application/json")
		if outcome == "unknown" {
			_, _ = io.WriteString(w, `{"unexpected":true}`)
		} else if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			_, _ = io.WriteString(w, `{"task":{"id":"reference-audio-provider-task","status":"pending"}}`)
		} else {
			_, _ = io.WriteString(w, `{"id":"reference-audio-provider-task"}`)
		}
	}))
	t.Cleanup(provider.Close)
	channel := model.Channel{Id: 8191, Type: constant.ChannelTypeSeedanceLink, Name: "fixture", Key: "provider-fixture-key", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, Models: "customer-audio", Group: "default", ModelMapping: common.GetPointer(`{"customer-audio":"seedance-2-0-mini"}`)}
	if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
		channel.ModelMapping = common.GetPointer(`{"customer-audio":"doubao-seedance-2-0-260128"}`)
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	require.NoError(t, db.Create(&channel).Error)
	content := []any{map[string]any{"type": "text", "text": "blue cup"}}
	for i := 0; i < 5; i++ {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://source.example/image.png"}, "role": "reference_image"})
	}
	ref := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(audio)
	if outcome == "base64" {
		ref = " \n" + ref + "\t"
	}
	if outcome == "https" {
		ref = "https://source.example/audio.wav?one=1&two=2"
	}
	if outcome == "file" || outcome == "raw_file" {
		ref = "file://audio"
	}
	if outcome == "raw_base64" {
		ref = base64.StdEncoding.EncodeToString(audio)
	}
	content = append(content, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": ref}, "role": "reference_audio"})
	if outcome == "repeated" {
		content = append(content, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": ref}, "role": "reference_audio"})
	}
	raw, err := common.Marshal(map[string]any{"model": "customer-audio", "duration": 5, "content": content})
	require.NoError(t, err)
	var input bytes.Buffer
	ct := "application/json"
	if outcome == "file" || outcome == "raw_file" {
		writer := multipart.NewWriter(&input)
		require.NoError(t, writer.WriteField("request", string(raw)))
		part, err := writer.CreateFormFile("audio", "audio.wav")
		require.NoError(t, err)
		_, err = part.Write(audio)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		ct = writer.FormDataContentType()
	} else {
		input.Write(raw)
	}
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(), middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", &input)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, req)
	var tasks []model.Task
	require.NoError(t, db.Find(&tasks).Error)
	var attempts []model.TaskCreateAttempt
	require.NoError(t, db.Find(&attempts).Error)
	var finalUser model.User
	require.NoError(t, db.First(&finalUser, user.Id).Error)
	var finalToken model.Token
	require.NoError(t, db.First(&finalToken, token.Id).Error)
	if outcome == "upload_failure" || outcome == "sign_failure" {
		assert.Equal(t, 503, response.Code, response.Body.String())
		assert.Zero(t, providerCalls)
		assert.Equal(t, 1, uploads)
		assert.Empty(t, tasks)
		assert.Empty(t, attempts)
		assert.Equal(t, 10000, finalUser.Quota)
		assert.Equal(t, 10000, finalToken.RemainQuota)
		return
	}
	require.Equal(t, 1, providerCalls, response.Body.String())
	require.Len(t, attempts, 1)
	assert.Equal(t, 9300, finalUser.Quota)
	if outcome == "https" {
		assert.Zero(t, uploads)
	} else if outcome == "repeated" {
		assert.Equal(t, 2, uploads)
	} else {
		assert.Equal(t, 1, uploads)
	}
	if outcome == "unknown" {
		require.Empty(t, tasks)
		assert.Equal(t, model.TaskCreateAttemptUnknown, attempts[0].Status)
		assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempts[0].BillingHoldState)
		recovered, err := service.RecoverUnknownTaskCreateAttempt(attempts[0].AttemptID, "reference-audio-provider-task", "", true, user.Id, "provider verified")
		require.NoError(t, err)
		require.NoError(t, db.Where("task_id = ?", recovered.PublicTaskID).Find(&tasks).Error)
		assert.Equal(t, 1, providerCalls)
	} else {
		assert.Equal(t, 200, response.Code, response.Body.String())
	}
	require.Len(t, tasks, 1)
	assert.Equal(t, sendingFacts, tasks[0].PrivateData.ReferenceAudio)
}
