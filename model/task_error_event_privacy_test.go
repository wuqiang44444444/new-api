package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskFailureEventsPersistOnlyPublicFailure(t *testing.T) {
	db := setupErrorEventTestDB(t)
	persisted := make(chan error, 2)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error {
		err := persistErrorEvent(e)
		persisted <- err
		return err
	})
	t.Cleanup(func() { clienterrlog.SetEventPersister(persistErrorEvent) })
	raw := `provider raw body: api_key=fixture-key https://upstream.example/image?signature=fixture-secret`
	submitTaskFailureEvent(&Task{TaskID: "task-privacy", UserId: 11, Status: TaskStatusFailure, FailReason: raw}, TaskStatusInProgress, "task_lifecycle")
	submitMidjourneyFailureEvent(&Midjourney{MjId: "mj-privacy", UserId: 11, Status: "FAILURE", FailReason: raw}, "IN_PROGRESS")
	for range 2 {
		select {
		case err := <-persisted:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("task failure event was not persisted")
		}
	}
	var rows []ErrorEvent
	require.NoError(t, db.Where("event_type = ?", clienterrlog.EventTaskFailure).Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Contains(t, row.Detail, "fail_reason")
		assert.NotContains(t, row.Detail, "fixture-key")
		assert.NotContains(t, row.Detail, "fixture-secret")
		assert.NotContains(t, row.Detail, "upstream.example")
		assert.NotContains(t, row.Detail, "provider raw body")
	}
}
