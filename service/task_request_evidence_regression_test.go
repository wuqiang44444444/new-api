package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceCreateResponseSurvivesPolling(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{"generate_audio":false}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	req, err := http.NewRequest(http.MethodPost, "https://provider.example/create", bytes.NewBufferString(`{"with_audio":false}`))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, req, info))
	_, err = io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"status":"created"}`)), Request: req}
	AttachTaskRequestEvidenceUpstreamResponse(c, resp)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	session := evidenceSessionFrom(c)
	task := &model.Task{TaskID: "task-regression", UserId: 7, ClientProtocol: "openai_video"}
	require.NoError(t, model.DB.Create(task).Error)
	AttachTaskRequestEvidenceTask(c, task)
	for _, body := range []string{`{"status":"running"}`, `{"status":"success"}`} {
		CaptureTaskPollingEvidence(task, "GET", "https://provider.example/task", 200, nil, []byte(body), nil)
	}
	events := evidenceEventRows(t, session.evidenceID)
	require.Len(t, events, 5)
	keys := map[string]bool{}
	for _, event := range events {
		assert.False(t, keys[EvidenceObjectKey(event.EvidenceId, event.Id)])
		keys[EvidenceObjectKey(event.EvidenceId, event.Id)] = true
		body := evidenceCapturedBody(t, event)
		assert.NotEmpty(t, body)
	}
	created := evidenceCapturedBody(t, events[2])
	assert.JSONEq(t, `{"status":"created"}`, string(created))
	assert.Equal(t, events[1].AttemptSeq, events[2].AttemptSeq)
	require.Error(t, GetTaskRequestEvidenceStore().Put(EvidenceObjectKey(events[2].EvidenceId, events[2].Id), []byte("replacement")))
}

type evidenceBrokenReader struct{ done bool }

func (r *evidenceBrokenReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("broken")
	}
	r.done = true
	return copy(p, "partial"), errors.New("broken")
}
func (*evidenceBrokenReader) Close() error { return nil }

func TestEvidenceResponsePreservesCapturedPrefix(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		limit                   int64
		early, broken           bool
	}{
		{name: "formatted json", body: "{\n \"ok\": true\n}", contentType: "application/json", limit: 1024},
		{name: "empty body", limit: 1024},
		{name: "whitespace without content type", body: " \r\n", limit: 1024},
		{name: "whitespace text", body: " \r\n", contentType: "text/plain", limit: 1024},
		{name: "whitespace json", body: " \r\n", contentType: "application/json", limit: 1024},
		{name: "whitespace event stream", body: " \r\n", contentType: "text/event-stream", limit: 1024},
		{name: "early close", body: "partial", limit: 1024, early: true},
		{name: "read error", limit: 1024, broken: true},
		{name: "single chunk over limit", body: "0123456789", limit: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			c := newEvidenceTestContext(t, []byte(`{}`))
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
			var origin io.ReadCloser = io.NopCloser(bytes.NewBufferString(tc.body))
			if tc.broken {
				origin = &evidenceBrokenReader{}
			}
			tee := &evidenceResponseBodyTee{session: evidenceSessionFrom(c), origin: origin, buffer: bytes.NewBuffer(nil), maxBytes: tc.limit, contentType: tc.contentType}
			if tc.early {
				_, _ = tee.Read(make([]byte, 2))
			} else {
				_, _ = io.ReadAll(tee)
			}
			require.NoError(t, tee.Close())
			require.NoError(t, tee.Close())
			events := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
			require.Len(t, events, 2)
			captured := evidenceCapturedBody(t, events[1])
			expected := tc.body
			if tc.early {
				expected = expected[:2]
			}
			if tc.broken {
				expected = "partial"
			}
			expected = expected[:min(int64(len(expected)), tc.limit)]
			assert.Equal(t, expected, string(captured))
			assert.Empty(t, events[1].Phase)

		})
	}
}

type unavailableEvidenceStore struct{}

func (unavailableEvidenceStore) PutEventBody(int64, int64, []byte) error {
	return errors.New("unavailable")
}

func (unavailableEvidenceStore) Put(string, []byte) error             { return errors.New("unavailable") }
func (unavailableEvidenceStore) Get(string) ([]byte, error)           { return nil, errors.New("unavailable") }
func (unavailableEvidenceStore) Delete(string) (int64, error)         { return 0, errors.New("unavailable") }
func (unavailableEvidenceStore) RemoveEmptyEvidenceDir(int64) error   { return nil }
func (unavailableEvidenceStore) EvidenceNamespaces() ([]int64, error) { return nil, nil }
func (unavailableEvidenceStore) WalkEvidenceFiles(context.Context, int64, func(EvidenceStoredFile) error) error {
	return errors.New("unavailable")
}

func TestEvidenceResponseStorageFailureLeavesUnavailableEvent(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
	evidenceObjectStore = unavailableEvidenceStore{}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewBufferString("audio"))}
	AttachTaskRequestEvidenceUpstreamResponse(c, resp)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "audio", string(body))
	events := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
	require.Len(t, events, 2)

	_, status := ReadEvidenceEventContent(events[1], false)
	assert.Equal(t, "read_failed", status)
}

func TestEvidenceInvalidWriteConfigurationDoesNotRejectBusiness(t *testing.T) {
	setupEvidenceTestEnv(t)
	config := system_setting.GetTaskRequestEvidenceConfig()
	config.WriteTimeoutSeconds = 0
	system_setting.SetTaskRequestEvidenceConfig(config)
	c := newEvidenceTestContext(t, []byte(`{}`))
	assert.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
}

func TestEvidenceRecoveredAttemptCannotSendOrRefund(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.TaskCreateAttempt{}))
	attempt := &model.TaskCreateAttempt{AttemptID: "attempt-test", Status: model.TaskCreateAttemptUnknown, BillingHoldState: model.TaskCreateAttemptBillingHeld}
	require.NoError(t, env.db.Create(attempt).Error)
	c := newEvidenceTestContext(t, []byte(`{}`))
	common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	evidenceObjectStore = unavailableEvidenceStore{}
	req, err := http.NewRequest("POST", "https://provider.example/create", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	require.ErrorIs(t, CaptureSouthboundTaskRequestEvidence(c, req, info), ErrTaskRequestEvidenceUnavailable)
	assert.True(t, info.SkipRequestRefund)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown))
}

func TestEvidenceRawPollingPreservesProviderFields(t *testing.T) {
	setupEvidenceTestEnv(t)
	task := &model.Task{TaskID: "task-raw", UserId: 7, ClientProtocol: "openai_video"}
	require.NoError(t, model.DB.Create(task).Error)
	req, err := http.NewRequest("GET", "https://provider.example/tasks/raw", nil)
	require.NoError(t, err)
	raw := `{"provider_field":"keep","status":"running","request_id":"upstream-1"}`
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(raw))}
	AttachRawTaskPollingEvidence(task, req, resp, nil)
	consumed, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, raw, string(consumed))
	evidence, exists, err := model.FindTaskRequestEvidenceByTaskID(task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	events := evidenceEventRows(t, evidence.Id)
	require.Len(t, events, 1)
	payload := evidenceCapturedBody(t, events[0])
	assert.JSONEq(t, raw, string(payload))
	assert.Equal(t, "polling", events[0].Stage)

}

func TestEvidenceClientResponseCapturesFinalPresentation(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	c.JSON(202, map[string]any{"id": "public-task", "status": "queued"})
	FinishTaskRequestEvidenceClientDelivery(c)
	FinishTaskRequestEvidenceClientDelivery(c)
	events := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
	require.Len(t, events, 2)
	assert.Equal(t, "client_delivery", events[1].Stage)
	assert.Equal(t, 202, events[1].StatusCode)

	payload := evidenceCapturedBody(t, events[1])
	assert.JSONEq(t, `{"id":"public-task","status":"queued"}`, string(payload))
}

func TestEvidenceConcurrentEventsKeepIndependentBodies(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	sqlDB, err := env.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	index := &model.TaskRequestEvidence{RequestID: "concurrent"}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	results := make(chan error, 2)
	for _, body := range []string{`{"stage":"poll"}`, `{"stage":"delivery"}`} {
		go func(payload string) {
			persistRawEvidenceEvent(&model.TaskRequestEvidenceEvent{EvidenceId: index.Id, Stage: model.TaskRequestEvidenceStagePolling, ContentType: "application/json"}, nil, nil, []byte(payload), "")
			results <- nil
		}(body)
	}
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	events := evidenceEventRows(t, index.Id)
	require.Len(t, events, 2)
	assert.NotEqual(t, events[0].Id, events[1].Id)
	bodies := map[string]bool{}
	for _, event := range events {
		body := evidenceCapturedBody(t, event)
		assert.NotEmpty(t, body)
		bodies[string(body)] = true
	}
	assert.True(t, bodies[`{"stage":"poll"}`])
	assert.True(t, bodies[`{"stage":"delivery"}`])
}

func TestEvidenceLateTaskIndexExpires(t *testing.T) {
	for _, delivery := range []bool{false, true} {
		t.Run(map[bool]string{false: "polling", true: "delivery"}[delivery], func(t *testing.T) {
			env := setupEvidenceTestEnv(t)
			config := system_setting.GetTaskRequestEvidenceConfig()
			config.RetentionDays = 7
			system_setting.SetTaskRequestEvidenceConfig(config)
			task := &model.Task{TaskID: "late-evidence", UserId: 7, ClientProtocol: "openai_video", Status: model.TaskStatusSuccess, Quota: 123}
			require.NoError(t, env.db.Create(task).Error)
			if delivery {
				CaptureTaskContentDeliveryEvidence(task.TaskID, 200, 12, nil)
			} else {
				CaptureTaskPollingEvidence(task, "GET", "https://provider.example/task", 200, nil, []byte(`{"status":"success"}`), nil)
			}
			evidence, exists, err := model.FindTaskRequestEvidenceByTaskID(task.TaskID)
			require.NoError(t, err)
			require.True(t, exists)
			assert.Equal(t, evidence.CreatedAt+7*24*60*60, evidence.RetainUntil)
			expired, err := model.TaskRequestEvidenceExpiredBodies(evidence.RetainUntil+1, 7*86400, 0, 0, 10)
			require.NoError(t, err)
			require.Len(t, expired, 1)
			assert.Equal(t, evidence.Id, expired[0].Id)
			saved, exists, err := model.GetByOnlyTaskId(task.TaskID)
			require.NoError(t, err)
			require.True(t, exists)
			assert.Equal(t, task.Status, saved.Status)
			assert.Equal(t, task.Quota, saved.Quota)
		})
	}
}
