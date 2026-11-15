package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceCleanupAlertDeliversOnceUntilRecovery(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	inbox := newLocalMailInbox(t)
	now := time.Now()
	at := now.Unix()
	health := &model.EvidenceCleanupHealth{
		Alerts: []string{"consecutive_failures", "cleanup_backlog"}, ConsecutiveFailures: 2, PendingIndexes: 3, LastFailedTaskID: "cleanup-test-failed",
	}
	ctx := context.Background()
	require.NoError(t, NotifyEvidenceCleanupHealth(ctx, at, health, false))
	first := requireLocalMail(t, inbox, "Evidence cleanup alert", "ops@example.com")
	assert.Contains(t, first.body, "consecutive_failures, cleanup_backlog")
	assert.Contains(t, first.body, "Pending indexes")
	assert.Contains(t, first.body, "cleanup-test-failed")
	assert.Contains(t, first.body, "requires Root access")
	// Flush/inspect the durable outbox only after the background sender has
	// written its known result, before resetting the test's global database.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var rows []model.ResourceAlertDelivery
		assert.NoError(c, db.Find(&rows).Error)
		if assert.Len(c, rows, 1) {
			assert.Equal(c, model.MailDeliveryAccepted, rows[0].Status)
		}
		assert.False(c, resourceAlertDeliveryQueued.Load())
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, recordEvidenceCleanupHealth(ctx, at+900, health, false))
	require.NoError(t, recordEvidenceCleanupHealth(ctx, at+1800, nil, true))
	var count int64
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	require.NoError(t, recordEvidenceCleanupHealth(ctx, at+2700, &model.EvidenceCleanupHealth{}, false))
	require.NoError(t, recordEvidenceCleanupHealth(ctx, at+3600, nil, true))
	require.NoError(t, deliverResourceAlerts(func() time.Time { return now.Add(time.Hour) }, common.SendEmailContext))
	second := requireLocalMail(t, inbox, "Evidence cleanup alert", "ops@example.com")
	assert.Contains(t, second.body, "Could not verify cleanup health")
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}
