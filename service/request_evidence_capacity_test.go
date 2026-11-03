package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeSixtyFourActiveConnectionsPreserveAllFourDirections(t *testing.T) {
	setupEvidenceTestEnv(t)
	beforeFailed, beforeDropped := imageEvidenceFailed.Load(), imageEvidenceDropped.Load()
	stages := []string{"realtime_north_receive", "realtime_southbound_send", "realtime_upstream_response", "realtime_client_delivery"}
	captures := make([]*ImageErrorEvidence, 64)
	for i := range captures {
		capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime", RequestID: fmt.Sprintf("connection-%d", i)})
		captures[i] = capture
		ctx := WithImageErrorEvidence(context.Background(), capture)
		for _, stage := range stages {
			ObserveRealtimeFrame(ctx, stage, websocket.TextMessage, []byte(capture.Index.RequestID+":"+stage), nil)
		}
	}
	// Every connection stays active until all four directions have submitted.
	for _, capture := range captures {
		capture.Finish(false)
	}
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{Num: 100})
	require.Len(t, indexes, 64)
	for _, index := range indexes {
		events := evidenceEventRows(t, index.Id)
		require.Len(t, events, len(stages))
		seen := make(map[string]bool)
		for _, event := range events {
			assert.False(t, seen[event.Stage], "a direction must not replace another")
			seen[event.Stage] = true
			content, status := ReadEvidenceEventContent(event, false)
			require.Equal(t, "available", status)
			require.Len(t, content.Messages, 1)
			require.NotNil(t, content.Messages[0].Text)
			assert.Equal(t, index.RequestID+":"+event.Stage, *content.Messages[0].Text)
		}
		for _, stage := range stages {
			assert.True(t, seen[stage])
		}
	}
	assert.Equal(t, beforeFailed, imageEvidenceFailed.Load())
	assert.Equal(t, beforeDropped, imageEvidenceDropped.Load())
}
