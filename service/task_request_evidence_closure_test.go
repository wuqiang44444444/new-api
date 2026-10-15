package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClosedEvidenceSkipsSnapshotsButStillChecksDurableSendPermit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
		allowed bool
	}{
		{"closed permits business", false, true},
		{"deleted permits business", true, true},
		{"closed does not authorize recovered attempt", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
			c := newEvidenceTestContext(t, []byte(`{"input":"hello"}`))
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
			id := evidenceSessionFrom(c).evidenceID
			require.NoError(t, model.RequestTaskRequestEvidenceCleanup(id, common.GetTimestamp()))
			if tc.missing {
				require.NoError(t, model.DB.Where("evidence_id = ?", id).Delete(&model.TaskRequestEvidenceEvent{}).Error)
				require.NoError(t, model.DB.Delete(&model.TaskRequestEvidence{}, id).Error)
			}
			status := model.TaskCreateAttemptSending
			if !tc.allowed {
				status = model.TaskCreateAttemptUnknown
			}
			attempt := &model.TaskCreateAttempt{AttemptID: "closure-permit", Status: status, BillingHoldState: model.TaskCreateAttemptBillingHeld}
			require.NoError(t, model.DB.Create(attempt).Error)
			common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
			request, err := http.NewRequest(http.MethodPost, "https://provider.example/create", bytes.NewBufferString(`{"input":"hello"}`))
			require.NoError(t, err)
			request.GetBody = func() (io.ReadCloser, error) {
				t.Error("closed evidence must not snapshot the business body")
				return nil, errors.New("snapshot unexpectedly invoked")
			}
			// No storage is needed after observation has ended.
			evidenceObjectStore = nil
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			err = CaptureSouthboundTaskRequestEvidence(c, request, info)
			if tc.allowed {
				require.NoError(t, err)
				assert.False(t, info.SkipRequestRefund)
			} else {
				require.ErrorIs(t, err, ErrTaskRequestEvidenceUnavailable)
				assert.True(t, info.SkipRequestRefund)
				assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown))
			}
			var count int64
			require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&count).Error)
			if tc.missing {
				assert.Zero(t, count)
			} else {
				assert.EqualValues(t, 1, count)
			}
		})
	}
}

func TestEvidenceCleanupDoesNotHideRealCaptureFailure(t *testing.T) {
	setupEvidenceTestEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, ContentType: "application/json"}
	processingErr := errors.New("test redaction processing failure")
	err := persistTaskEvidenceBodyUsing(event, []byte(`{}`), TaskRequestEvidenceSourceSouth, func([]byte, string) ([]byte, error) {
		require.NoError(t, model.RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
		require.NoError(t, model.MarkTaskRequestEvidenceBodyExpired(index.Id))
		_, err := model.DeleteClosedEvidenceEventBatch(index.Id, 500)
		require.NoError(t, err)
		return nil, processingErr
	})
	require.ErrorIs(t, err, ErrTaskRequestEvidenceUnavailable)
	assert.ErrorIs(t, err, processingErr)
	assert.ErrorIs(t, err, model.ErrTaskRequestEvidenceBodyClosed)
	var failure *TaskRequestEvidenceError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, TaskRequestEvidenceCategoryProcessing, failure.Category)
}

func TestEvidenceCleanupAfterPublicationDoesNotRejectCompletion(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, ContentType: "application/json", Phase: model.TaskRequestEvidencePhaseCompleted, Complete: true}
	store := GetTaskRequestEvidenceStore()
	// Execute cleanup at the completion boundary after the body was published.
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("cleanup-before-evidence-completion", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "task_request_evidence_events" {
			return
		}
		cleanup := tx.Session(&gorm.Session{NewDB: true, SkipDefaultTransaction: true})
		require.NoError(t, cleanup.Model(&model.TaskRequestEvidence{}).Where("id = ?", index.Id).Updates(map[string]any{"cleanup_requested_at": common.GetTimestamp(), "body_expired": true}).Error)
		require.NoError(t, cleanup.Where("evidence_id = ?", index.Id).Delete(&model.TaskRequestEvidenceEvent{}).Error)
		_, err := store.Delete(EvidenceObjectKey(index.Id, event.Id))
		require.NoError(t, err)
		require.NoError(t, store.RemoveEmptyEvidenceDir(index.Id))
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove("cleanup-before-evidence-completion")) })
	require.NoError(t, persistTaskEvidenceBody(event, []byte(`{"prompt":"media"}`), TaskRequestEvidenceSourceSouth))
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&count).Error)
	assert.Zero(t, count)
	entries, err := os.ReadDir(env.storeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
