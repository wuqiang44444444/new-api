package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoFailureEventsCarryUnifiedPublicProjection(t *testing.T) {
	db := setupErrorEventTestDB(t)
	persisted := make(chan error, 3)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error {
		err := persistErrorEvent(e)
		persisted <- err
		return err
	})
	t.Cleanup(func() { clienterrlog.SetEventPersister(persistErrorEvent) })
	message := "输入内容未通过安全审核 (input content failed the upstream safety review)"
	data, err := common.Marshal(map[string]any{"status": "failed", "error": map[string]string{"code": "AuditSubmitIllegal", "message": message}})
	require.NoError(t, err)
	submitTaskFailureEvent(&Task{TaskID: "task-video-event", UserId: 11, ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusFailure, FailReason: message, Data: data, Properties: Properties{OriginModelName: "customer-vidu"}}, TaskStatusInProgress, "task_lifecycle")
	submitTaskFailureEvent(&Task{TaskID: "task-batch-event", Platform: constant.TaskPlatformAzureBatch, Status: TaskStatusFailure, FailReason: "row failed"}, TaskStatusInProgress, "batch_settlement")
	submitTaskFailureEvent(&Task{TaskID: "task-expired-event", ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusExpired, FailReason: "expired"}, TaskStatusInProgress, "task_lifecycle")
	for range 3 {
		select {
		case err := <-persisted:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("task failure event was not persisted")
		}
	}
	var rows []ErrorEvent
	require.NoError(t, db.Where("event_type = ?", clienterrlog.EventTaskFailure).Order("id").Find(&rows).Error)
	require.Len(t, rows, 3)
	video, batch, expired := rows[0], rows[1], rows[2]
	assert.Equal(t, "AuditSubmitIllegal", video.PublicCode)
	var detail map[string]string
	require.NoError(t, common.Unmarshal([]byte(video.Detail), &detail))
	assert.Equal(t, message, detail["fail_reason"])
	assert.Empty(t, batch.PublicCode, "non-video tasks keep their own contract without a video code")
	assert.Contains(t, batch.Detail, "fail_reason")
	assert.Empty(t, expired.PublicCode, "expired tasks keep the existing expired contract")
	require.NoError(t, common.Unmarshal([]byte(expired.Detail), &detail))
	assert.Equal(t, "expired", detail["fail_reason"])
}
