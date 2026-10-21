package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
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

func TestDurableSendingPermissionIsCheckedAfterEvidenceFactsUpdate(t *testing.T) {
	setupEvidenceTestEnv(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
	attempt := &model.TaskCreateAttempt{AttemptID: "recovered-during-evidence-update", Status: model.TaskCreateAttemptSending, BillingHoldState: model.TaskCreateAttemptBillingHeld}
	require.NoError(t, model.DB.Create(attempt).Error)
	c := newEvidenceTestContext(t, []byte(`{}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
	common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("recover-during-evidence-update", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "task_request_evidences" {
			return
		}
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Model(&model.TaskCreateAttempt{}).Where("id = ?", attempt.ID).Update("status", model.TaskCreateAttemptUnknown).Error)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove("recover-during-evidence-update")) })
	req, err := http.NewRequest("POST", "https://provider.test/create", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	require.ErrorIs(t, CaptureSouthboundTaskRequestEvidence(c, req, info), ErrTaskRequestEvidenceUnavailable)
	assert.True(t, info.SkipRequestRefund)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown))
}
