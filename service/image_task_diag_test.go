package service

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replaceImageDiagSink(t *testing.T, sink *clienterrlog.Sink) {
	t.Helper()
	previous := imageTaskDiagSink
	t.Cleanup(func() { imageTaskDiagSink = previous })
	imageTaskDiagSink = sink
}

func diagFixtureTask() *model.Task {
	task := &model.Task{TaskID: "task_diag_fixture", ChannelId: 42, Platform: model.ImageTaskPlatform(constant.ChannelTypeOpenAI)}
	task.PrivateData.ImageTask = &model.TaskImageExecutionData{ChannelType: constant.ChannelTypeOpenAI, Operation: "generations"}
	return task
}

func waitDiagEvent(t *testing.T, delivered chan clienterrlog.Event) clienterrlog.Event {
	t.Helper()
	select {
	case event := <-delivered:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("diagnostic event was not delivered")
		return clienterrlog.Event{}
	}
}

func TestImageTaskDiagSubmissionNeverBlocks(t *testing.T) {
	release := make(chan struct{})
	delivered := make(chan clienterrlog.Event, 8)
	sink := clienterrlog.NewSink(t.Context(), 8, func(e clienterrlog.Event) error {
		<-release
		delivered <- e
		return nil
	})
	replaceImageDiagSink(t, sink)

	done := make(chan struct{})
	go func() {
		emitImageTaskExecutionDiag(diagFixtureTask(), model.TaskStatusFailure, ImageTaskExecution{
			Outcome: ImageTaskOutcomeFailure, FailureCode: "provider_rejected", UpstreamStatus: 400, ProviderRequestID: "req-1",
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("diagnostic submission blocked on a stalled writer")
	}
	close(release)
	event := waitDiagEvent(t, delivered)
	assert.Contains(t, event.Message, "event=image_task_execution_diag")
	assert.Contains(t, event.Message, "stage=provider_rejected")
	assert.Contains(t, event.Message, "upstream_status=400")
	assert.Contains(t, event.Message, "provider_request_id=req-1")
	assert.Contains(t, event.Message, "trusted=true")
}

func TestImageTaskDiagOverflowDropsWithoutBlocking(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	sink := clienterrlog.NewSink(t.Context(), 1, func(e clienterrlog.Event) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	})
	replaceImageDiagSink(t, sink)

	emitImageTaskExecutionDiag(diagFixtureTask(), model.TaskStatusFailure, ImageTaskExecution{FailureCode: "provider_rejected"})
	<-started
	emitImageTaskExecutionDiag(diagFixtureTask(), model.TaskStatusFailure, ImageTaskExecution{FailureCode: "provider_rejected"})
	emitImageTaskExecutionDiag(diagFixtureTask(), model.TaskStatusFailure, ImageTaskExecution{FailureCode: "provider_rejected"})
	health := ImageTaskDiagHealth()
	assert.GreaterOrEqual(t, health.Dropped, uint64(1))
	assert.LessOrEqual(t, health.Queued, 1)
	close(release)
}

func TestImageTaskDiagUnknownKeepsSanitizedSingleLineFacts(t *testing.T) {
	delivered := make(chan clienterrlog.Event, 8)
	sink := clienterrlog.NewSink(t.Context(), 8, func(e clienterrlog.Event) error {
		delivered <- e
		return nil
	})
	replaceImageDiagSink(t, sink)

	task := diagFixtureTask()
	task.PrivateData.ImageTask.Operation = "edits\nmultipart"
	emitImageTaskExecutionDiag(task, model.TaskStatusReconciliationRequired, ImageTaskExecution{
		Outcome: ImageTaskOutcomeUnknown, FailureCode: "native_image_outcome_unknown",
		UpstreamStatus: 502, ProviderRequestID: "line1\nline2 extra", ViolationMarker: true,
	})
	event := waitDiagEvent(t, delivered)
	assert.Contains(t, event.Message, "status=reconciliation_required")
	assert.Contains(t, event.Message, "trusted=false")
	assert.Contains(t, event.Message, "stage=native_image_outcome_unknown")
	assert.Contains(t, event.Message, "upstream_status=502")
	assert.Contains(t, event.Message, "violation_marker=true")
	assert.Contains(t, event.Message, "operation=editsmultipart")
	assert.Contains(t, event.Message, "provider_request_id=line1line2_extra")
	assert.NotContains(t, event.Message, "\n")
	require.True(t, event.RequestID != "")
}

func TestImageTaskDiagHealthReportsWriterFailureSeparately(t *testing.T) {
	before := clienterrlog.CurrentHealth()
	sink := clienterrlog.NewSink(t.Context(), 1, func(clienterrlog.Event) error { return errors.New("writer unavailable") })
	replaceImageDiagSink(t, sink)
	emitImageTaskExecutionDiag(diagFixtureTask(), model.TaskStatusFailure, ImageTaskExecution{FailureCode: "provider_rejected"})
	require.Eventually(t, func() bool { return ImageTaskDiagHealth().Failed == 1 }, time.Second, time.Millisecond)
	assert.Equal(t, before, clienterrlog.CurrentHealth(), "task execution logs do not count as HTTP 4xx events")
}
