package service

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceFailureCannotAuthorizeUnknownOrRefundHeldAttempt(t *testing.T) {
	for _, status := range []model.TaskCreateAttemptStatus{model.TaskCreateAttemptSending, model.TaskCreateAttemptUnknown} {
		t.Run(string(status), func(t *testing.T) {
			setupEvidenceTestEnv(t)
			require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
			attempt := &model.TaskCreateAttempt{AttemptID: "durable-fixture", Status: status, BillingHoldState: model.TaskCreateAttemptBillingHeld}
			require.NoError(t, model.DB.Create(attempt).Error)
			c := newEvidenceTestContext(t, []byte(`{}`))
			common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
			evidenceObjectStore = nil
			req, err := http.NewRequest("POST", "https://provider.invalid/create", bytes.NewReader([]byte(`{}`)))
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			err = CaptureSouthboundTaskRequestEvidence(c, req, info)
			if status == model.TaskCreateAttemptSending {
				require.NoError(t, err)
				assert.False(t, info.SkipRequestRefund)
			} else {
				require.ErrorIs(t, err, ErrTaskRequestEvidenceUnavailable)
				assert.True(t, info.SkipRequestRefund)
				assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown))
			}
			var stored model.TaskCreateAttempt
			require.NoError(t, model.DB.First(&stored, attempt.ID).Error)
			assert.Equal(t, status, stored.Status)
			assert.Equal(t, model.TaskCreateAttemptBillingHeld, stored.BillingHoldState)
		})
	}
}
