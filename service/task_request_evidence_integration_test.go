package service

import (
	"bytes"
	"github.com/QuantumNous/new-api/constant"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 端到端契约测试：真实 sqlite + 真实加密存储 + gin 上下文 + 假 Provider
// HTTP 回环。验证方案 §6.7 的核心链路：北向采集（脱敏、完整性）→
// 南向采集（最终正文、履约事实）→ 响应有界捕获 → 任务关联 →
// 权限视图（遮盖/原始），以及证据不可用时的发送前拒绝。

type evidenceTestEnv struct {
	db       *gorm.DB
	storeDir string
	oldDB    *gorm.DB
}

func setupEvidenceTestEnv(t *testing.T) *evidenceTestEnv {
	t.Helper()
	env := &evidenceTestEnv{}

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence-test.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.TaskRequestEvidence{},
		&model.TaskRequestEvidenceEvent{},
		&model.TaskRequestEvidenceAccessLog{},
	))
	env.db = db
	env.oldDB = model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = env.oldDB })

	env.storeDir = t.TempDir()
	evidenceConfig := system_setting.TaskRequestEvidenceConfig{
		Enabled:             true,
		RetentionDays:       7,
		StorageDir:          env.storeDir,
		EncryptionKeyHex:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		MaxBodyBytes:        1 << 20,
		MaxResponseBytes:    1 << 20,
		WriteTimeoutSeconds: 5,
	}
	system_setting.SetTaskRequestEvidenceConfig(evidenceConfig)
	require.NoError(t, InitTaskRequestEvidenceStore(evidenceConfig))
	t.Cleanup(func() {
		evidenceObjectStore = nil
		system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	})
	return env
}

func newEvidenceTestContext(t *testing.T, body []byte) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech?model=tts-1", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer sk-client-token")
	c.Set(common.RequestIdKey, "req-evidence-e2e-1")
	c.Set("id", 7)
	c.Set("token_id", 42)
	c.Set("app_id", 3)
	return c
}

func evidenceEventRows(t *testing.T, evidenceId int64) []*model.TaskRequestEvidenceEvent {
	t.Helper()
	events, err := model.ListTaskRequestEvidenceEvents(evidenceId)
	require.NoError(t, err)
	return events
}

// evidenceCapturedBody reads the same selected content contract as the controller.
func evidenceCapturedBody(t *testing.T, event *model.TaskRequestEvidenceEvent) []byte {
	t.Helper()
	content, status := ReadEvidenceEventContent(event, false)
	require.Equal(t, "available", status)
	return content.Body
}

func TestEvidenceEndToEndCaptureFlow(t *testing.T) {
	setupEvidenceTestEnv(t)
	raw := []byte(`{ "input":"你好", "api_key":"synthetic", "id":9007199254740993, "input":"duplicate" }`)
	c := newEvidenceTestContext(t, raw)
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
	id := evidenceSessionFrom(c).evidenceID
	rows := evidenceEventRows(t, id)
	require.Len(t, rows, 1)
	assert.Equal(t, raw, evidenceCapturedBody(t, rows[0]))
	req, err := http.NewRequest("POST", "https://provider.test/create", bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer synthetic-upstream")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}}
	info.OriginModelName = "client-model"
	info.UpstreamModelName = "provider-model"
	require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, req, info))
	sent, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	assert.Equal(t, raw, sent)
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: req}
	AttachTaskRequestEvidenceUpstreamResponse(c, resp)
	received, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, raw, received)
	rows = evidenceEventRows(t, id)
	require.Len(t, rows, 3)
	for _, event := range rows {
		assert.Equal(t, raw, evidenceCapturedBody(t, event))
		assert.Empty(t, event.Phase)
		assert.Empty(t, event.Sha256)
		assert.Empty(t, event.Detail)
	}
	content, status := ReadEvidenceEventContent(rows[1], false)
	require.Equal(t, "available", status)
	assert.Equal(t, []string{"Bearer synthetic-upstream"}, content.Meta.Headers["Authorization"])
}

// TestEvidenceCaptureSouthboundNilTaskRelayInfo 回归音频等无任务路径：
// RelayInfo.TaskRelayInfo 为 nil 时，ClientProtocol 经嵌入指针提升访问，
// 无守卫会空指针崩溃。此测试钉住该边界；其余提升字段（ChannelType 等）
// 经 *ChannelMeta 提升，由 doRequest 调用点的 InitChannelMeta 合同保证非空。
func TestEvidenceCaptureSouthboundNilTaskRelayInfo(t *testing.T) {
	_ = setupEvidenceTestEnv(t)

	c := newEvidenceTestContext(t, []byte(`{"model":"tts-1","input":"hi"}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}}
	info.OriginModelName = "tts-1"
	info.UpstreamModelName = "tts-1-hd"
	require.Nil(t, info.TaskRelayInfo, "precondition: audio path has no TaskRelayInfo")

	outbound, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/v1/audio/speech",
		bytes.NewReader([]byte(`{"model":"tts-1-hd","input":"hi"}`)))
	require.NoError(t, err)

	// 无任务路径不得崩溃：证据正常采集，嵌入指针字段安全缺席。
	require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, outbound, info))

	session := evidenceSessionFrom(c)
	require.NotNil(t, session)
	row, exists, err := model.GetTaskRequestEvidenceById(session.evidenceID)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "tts-1", row.ClientModel)
	assert.Equal(t, "tts-1-hd", row.UpstreamModel)
	assert.Equal(t, 5, row.ChannelID)
	assert.Empty(t, row.ClientProtocol)
}

func TestEvidenceFailureAndLimitsDoNotChangeBusinessBody(t *testing.T) {
	for _, tc := range []string{"missing store", "failed store", "capture limit", "malformed"} {
		t.Run(tc, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			raw := []byte(`{"api_key":"synthetic`)
			if tc == "capture limit" {
				config := system_setting.GetTaskRequestEvidenceConfig()
				config.MaxBodyBytes = 8
				system_setting.SetTaskRequestEvidenceConfig(config)
			}
			if tc == "missing store" {
				evidenceObjectStore = nil
			}
			if tc == "failed store" {
				evidenceObjectStore = unavailableEvidenceStore{}
			}
			c := newEvidenceTestContext(t, raw)
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
			req, err := http.NewRequest("POST", "https://provider.test/create", bytes.NewReader(raw))
			require.NoError(t, err)
			require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}))
			sent, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, req.Body.Close())
			assert.Equal(t, raw, sent)
			rows := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
			require.Len(t, rows, 2)
			for _, event := range rows {
				content, status := ReadEvidenceEventContent(event, false)
				switch tc {
				case "missing store":
					assert.Equal(t, "storage_unavailable", status)
				case "failed store":
					assert.Equal(t, "read_failed", status)
				default:
					require.Equal(t, "available", status)
					expected := raw
					if tc == "capture limit" {
						expected = raw[:8]
					}
					assert.Equal(t, expected, content.Body)
				}
			}
		})
	}
}

func TestEvidenceTaskAssociationAndViews(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	_ = env

	c := newEvidenceTestContext(t, []byte(`{"model":"seedance-1","prompt":"a cat"}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	session := evidenceSessionFrom(c)
	require.NotNil(t, session)

	task := &model.Task{
		TaskID:    "task_evidence_e2e",
		UserId:    7,
		AppID:     3,
		ChannelId: 5,
	}
	require.NoError(t, env.db.Create(task).Error)
	AttachTaskRequestEvidenceTask(c, task)

	evidenceRow, exists, err := model.GetTaskRequestEvidenceById(session.evidenceID)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "task_evidence_e2e", evidenceRow.TaskID)

	require.NoError(t, model.CreateTaskRequestEvidenceEvent(&model.TaskRequestEvidenceEvent{
		EvidenceId: session.evidenceID,
		Seq:        9,
		Stage:      model.TaskRequestEvidenceStageUpstreamResponse,
		ObjectKey:  "",
	}))
	events := evidenceEventRows(t, session.evidenceID)
	require.NotEmpty(t, events)
}

func TestEvidenceFailuresDoNotChangeProviderCallOrHeldFunds(t *testing.T) {
	for _, scenario := range []string{"missing store", "failed store", "index database", "event database", "capture limit"} {
		t.Run(scenario, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
			attempt := &model.TaskCreateAttempt{AttemptID: "evidence-independent-send", Status: model.TaskCreateAttemptSending, BillingHoldState: model.TaskCreateAttemptBillingHeld}
			require.NoError(t, model.DB.Create(attempt).Error)
			switch scenario {
			case "missing store":
				evidenceObjectStore = nil
			case "failed store":
				evidenceObjectStore = unavailableEvidenceStore{}
			case "index database":
				require.NoError(t, model.DB.Migrator().DropTable(&model.TaskRequestEvidence{}))
			case "event database":
				require.NoError(t, model.DB.Migrator().DropTable(&model.TaskRequestEvidenceEvent{}))
			case "capture limit":
				config := system_setting.GetTaskRequestEvidenceConfig()
				config.MaxBodyBytes = 4
				config.MaxResponseBytes = 4
				system_setting.SetTaskRequestEvidenceConfig(config)
			}
			raw := []byte(`{"input":"complete business payload"}`)
			c := newEvidenceTestContext(t, raw)
			common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, raw, body)
				w.WriteHeader(202)
				_, _ = w.Write([]byte(`{"id":"provider-task"}`))
			}))
			defer provider.Close()
			req, err := http.NewRequest("POST", provider.URL, bytes.NewReader(raw))
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, req, info))
			response, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			AttachTaskRequestEvidenceUpstreamResponse(c, response)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.Equal(t, 202, response.StatusCode)
			assert.Equal(t, `{"id":"provider-task"}`, string(body))
			assert.EqualValues(t, 1, calls.Load())
			var unchanged model.TaskCreateAttempt
			require.NoError(t, model.DB.First(&unchanged, attempt.ID).Error)
			assert.Equal(t, attempt.Status, unchanged.Status)
			assert.Equal(t, attempt.BillingHoldState, unchanged.BillingHoldState)
			assert.False(t, info.SkipRequestRefund)
		})
	}
}

func TestEvidencePreservesMultipartBase64SSEAndRepeatedQueryValues(t *testing.T) {
	var multipartBody bytes.Buffer
	form := multipart.NewWriter(&multipartBody)
	require.NoError(t, form.SetBoundary("evidence-fixture"))
	require.NoError(t, form.WriteField("request", `{"token":"synthetic","audio_url":"file://audio"}`))
	part, err := form.CreateFormFile("audio", "sample.wav")
	require.NoError(t, err)
	_, err = part.Write([]byte{0xff, 0x00, 0x01})
	require.NoError(t, err)
	require.NoError(t, form.Close())
	for _, tc := range []struct {
		name, kind string
		body       []byte
	}{
		{"multipart", form.FormDataContentType(), multipartBody.Bytes()},
		{"base64", "application/json", []byte(`{"image":"data:image/png;base64,AQIDBA==","api_key":"synthetic"}`)},
		{"sse", "text/event-stream", []byte("data: {\"url\":\"https:\\/\\/fixture.test/?sig=synthetic\"}\n\ndata: [DONE]\n\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			c := newEvidenceTestContext(t, tc.body)
			c.Request.Header.Set("Content-Type", tc.kind)
			c.Request.Header["X-Fixture"] = []string{"first", "second"}
			c.Request.URL.RawQuery = "tag=one&tag=two&key=synthetic"
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
			rows := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
			require.Len(t, rows, 1)
			content, status := ReadEvidenceEventContent(rows[0], false)
			require.Equal(t, "available", status)
			assert.Equal(t, tc.body, content.Body)
			assert.Equal(t, []string{"first", "second"}, content.Meta.Headers["X-Fixture"])
			assert.Equal(t, []string{"one", "two"}, content.Meta.Query["tag"])
			assert.Equal(t, []string{"synthetic"}, content.Meta.Query["key"])
		})
	}
}
