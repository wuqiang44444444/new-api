package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxNexhinaCreateAuthRejectionReleasesOnlyVerifiedHold(t *testing.T) {
	const hold = 600_000 // Frozen 15-second maximum at $0.08 and 500,000 quota/USD.
	const refusal = `{"error":{"code":"","type":"nexhina_error","message":"Invalid provider credential"}}`
	for _, tc := range []struct {
		name       string
		body       string
		status     model.TaskCreateAttemptStatus
		holdState  model.TaskCreateAttemptBillingHoldState
		quotaSpent int
	}{
		{"verified refusal", refusal, model.TaskCreateAttemptRejected, model.TaskCreateAttemptBillingReleased, 0},
		{"task ID contradicts refusal", `{"task_id":"maybe-accepted",` + refusal[1:], model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
		{"unverified 401", `<html>Unauthorized</html>`, model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
		{"incomplete response within read limit", refusal + strings.Repeat(" ", 32<<10) + "trailing content", model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr": `{"customer-video":"tier(\"nexhina\", u(\"output_seconds\") * 0.08)"}`,
			}))
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/hailuo/v2/video_generation", r.URL.Path)
				fx.createCalls.Add(1)
				var attempt model.TaskCreateAttempt
				if assert.NoError(t, fx.db.First(&attempt).Error) {
					assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
					assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
					assert.Equal(t, hold, attempt.HeldQuota)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(provider.Close)
			require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Updates(map[string]any{
				"base_url": provider.URL,
				"settings": `{"video_upstream_protocol":"nexhina_hailuo_v2","asset_upstream_protocol":"none"}`,
			}).Error)
			response := fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`)
			assert.NotContains(t, response, "maybe-accepted")
			var attempt model.TaskCreateAttempt
			require.NoError(t, fx.db.First(&attempt).Error)
			assert.Equal(t, tc.status, attempt.Status)
			assert.Equal(t, tc.holdState, attempt.BillingHoldState)
			assert.Equal(t, seedanceFundsInitialQuota-tc.quotaSpent, fx.userQuota())
			remaining, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-tc.quotaSpent, remaining)
			assert.Equal(t, tc.quotaSpent, used)
			var tasks int64
			require.NoError(t, fx.db.Model(&model.Task{}).Count(&tasks).Error)
			assert.Zero(t, tasks)
			assert.EqualValues(t, 1, fx.createCalls.Load())
		})
	}
}
