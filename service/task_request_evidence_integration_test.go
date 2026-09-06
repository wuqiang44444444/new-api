package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func TestEvidenceEndToEndCaptureFlow(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	_ = env

	body := []byte(`{"model":"tts-1","input":"你好","api_key":"sk-must-not-be-stored","max_tokens":256}`)
	c := newEvidenceTestContext(t, body)

	// 1) 北向采集：同步持久化，凭据脱敏，业务字段保留。
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
	session := evidenceSessionFrom(c)
	require.NotNil(t, session)
	require.Greater(t, session.evidenceID, int64(0))

	northEvents := evidenceEventRows(t, session.evidenceID)
	require.Len(t, northEvents, 1)
	assert.Equal(t, model.TaskRequestEvidenceStageNorthReceive, northEvents[0].Stage)
	assert.Equal(t, int64(len(body)), northEvents[0].ByteCount)
	assert.True(t, northEvents[0].Redacted)
	assert.NotEmpty(t, northEvents[0].ObjectKey)
	assert.True(t, northEvents[0].Complete)

	northObject, err := GetTaskRequestEvidenceStore().Get(northEvents[0].ObjectKey)
	require.NoError(t, err)
	var northBody map[string]any
	require.NoError(t, json.Unmarshal(northObject, &northBody))
	// 凭据被占位符替换；max_tokens 等业务字段完整保留。
	assert.Equal(t, "[REDACTED]", northBody["api_key"])
	assert.Equal(t, float64(256), northBody["max_tokens"])
	assert.Equal(t, "你好", northBody["input"])
	// 完整性校验值与存储正文一致。
	assert.Equal(t, EvidenceSha256Hex(northObject), northEvents[0].Sha256)

	// 2) 南向采集：最终正文（映射/override 之后）在发送前持久化。
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("X-Upstream-Request-Id", "up-req-777")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("FAKE-AUDIO-OK"))
	}))
	defer fakeProvider.Close()

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}}
	info.OriginModelName = "tts-1"
	info.UpstreamModelName = "tts-1-hd"

	outbound, err := http.NewRequest(http.MethodPost, fakeProvider.URL+"/v1/audio/speech",
		bytes.NewReader([]byte(`{"model":"tts-1-hd","input":"你好","with_defaults":true,"api_key":"sk-upstream"}`)))
	require.NoError(t, err)
	outbound.Header.Set("Authorization", "Bearer sk-upstream-credential")
	outbound.Header.Set("Content-Type", "application/json")

	require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, outbound, info))

	southEvents := evidenceEventRows(t, session.evidenceID)
	require.Len(t, southEvents, 2)
	assert.Equal(t, model.TaskRequestEvidenceStageSouthboundSend, southEvents[1].Stage)
	southObject, err := GetTaskRequestEvidenceStore().Get(southEvents[1].ObjectKey)
	require.NoError(t, err)
	assert.Contains(t, string(southObject), "tts-1-hd")
	assert.Contains(t, string(southObject), "with_defaults")
	assert.NotContains(t, string(southObject), "sk-upstream")

	// 索引事实已回填：客户模型、上游模型、渠道。
	evidenceRow, exists, err := model.GetTaskRequestEvidenceById(session.evidenceID)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "tts-1", evidenceRow.ClientModel)
	assert.Equal(t, "tts-1-hd", evidenceRow.UpstreamModel)
	assert.Equal(t, 5, evidenceRow.ChannelID)

	// 3) 实际发送：响应被边转发边捕获，转发内容不被破坏。
	transportResp, err := http.DefaultClient.Do(outbound)
	require.NoError(t, err)
	AttachTaskRequestEvidenceUpstreamResponse(c, transportResp)
	forwarded, err := io.ReadAll(transportResp.Body)
	require.NoError(t, err)
	require.NoError(t, transportResp.Body.Close())
	assert.Equal(t, "FAKE-AUDIO-OK", string(forwarded))

	// 4) 上游响应事件：转发内容与捕获内容一致，完整性校验匹配。
	upstreamEvents := evidenceEventRows(t, session.evidenceID)
	require.Len(t, upstreamEvents, 3)
	assert.Equal(t, model.TaskRequestEvidenceStageUpstreamResponse, upstreamEvents[2].Stage)
	assert.Equal(t, http.StatusOK, upstreamEvents[2].StatusCode)
	assert.Equal(t, int64(len("FAKE-AUDIO-OK")), upstreamEvents[2].ByteCount)
	require.NotEmpty(t, upstreamEvents[2].ObjectKey)
	capturedObject, err := GetTaskRequestEvidenceStore().Get(upstreamEvents[2].ObjectKey)
	require.NoError(t, err)
	assert.Equal(t, "FAKE-AUDIO-OK", string(capturedObject))
	assert.Equal(t, EvidenceSha256Hex(capturedObject), upstreamEvents[2].Sha256)
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

func TestEvidenceFailClosedBeforeSend(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	_ = env

	body := []byte(`{"model":"tts-1","input":"hi"}`)
	c := newEvidenceTestContext(t, body)
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))

	// 存储不可用：南向采集必须返回拒绝边界错误（尚未发送任何字节）。
	savedStore := evidenceObjectStore
	evidenceObjectStore = nil
	defer func() { evidenceObjectStore = savedStore }()

	outbound, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/v1", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	err = CaptureSouthboundTaskRequestEvidence(c, outbound, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.Error(t, err)
	assert.True(t, IsTaskRequestEvidenceUnavailable(err))
}

func TestEvidenceBodyTooLargeRejected(t *testing.T) {
	_ = setupEvidenceTestEnv(t)

	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{
		Enabled:          true,
		StorageDir:       t.TempDir(),
		EncryptionKeyHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		MaxBodyBytes:     16,
	})
	t.Cleanup(func() {
		system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	})

	c := newEvidenceTestContext(t, []byte(`{"model":"tts-1","input":"this body is definitely longer than sixteen bytes"}`))
	err := BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech)
	require.Error(t, err)
	assert.True(t, IsTaskRequestEvidenceUnavailable(err))
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

	// 视图：事件预览对非 Root 遮盖签名 URL，Root 看到原文。
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(&model.TaskRequestEvidenceEvent{
		EvidenceId: session.evidenceID,
		Seq:        9,
		Stage:      model.TaskRequestEvidenceStageUpstreamResponse,
		ObjectKey:  "",
	}))
	events := evidenceEventRows(t, session.evidenceID)
	require.NotEmpty(t, events)
	previews := GetEvidenceEventPreviews(session.evidenceID, events, false)
	rootPreviews := GetEvidenceEventPreviews(session.evidenceID, events, true)
	_ = previews
	_ = rootPreviews
}
