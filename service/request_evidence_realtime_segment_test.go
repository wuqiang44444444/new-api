package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRealtimeSegmentsKeepOpaqueMessagesDirectionsAndSealWithoutEmptyTail(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ctx := WithImageErrorEvidence(context.Background(), capture)
	first := "\n{broken JSON\x00\n"
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, []byte(first), nil)
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, nil, nil)
	ObserveRealtimeFrame(ctx, "realtime_southbound_send", websocket.TextMessage, []byte("other direction"), nil)
	record := capture.realtimeRecord("realtime_north_receive")
	// Same sealing callback as the worker's short-window tick; no sleep/timing assertion.
	record.rotate(record.piece, record.piece.sealAt)
	drainImageEvidence(t)
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, []byte("next segment"), errors.New("frame failure"))
	record.rotate(record.piece, record.piece.sealAt)
	drainImageEvidence(t)
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 3, "two north segments plus one south segment, no empty tail")
	north := 0
	for _, event := range events {
		content, status := ReadEvidenceEventContent(event, false)
		require.Equal(t, "available", status)
		if event.Stage == "realtime_southbound_send" {
			require.Len(t, content.Messages, 1)
			assert.Equal(t, "other direction", *content.Messages[0].Text)
			continue
		}
		if north == 0 {
			require.Len(t, content.Messages, 2)
			assert.Equal(t, first, *content.Messages[0].Text)
			assert.Equal(t, "", *content.Messages[1].Text, "empty message has a boundary")
			assert.Positive(t, content.Messages[0].CreatedAtMS)
		} else {
			require.Len(t, content.Messages, 1)
			assert.Equal(t, "next segment", *content.Messages[0].Text)
			assert.Equal(t, "frame failure", content.Messages[0].Error)
		}
		north++
	}
	assert.Equal(t, 2, north)
}

func TestRealtimeLargeMessageRetainsFullBodyAndOnlyLimitsPreview(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	body := bytes.Repeat([]byte("x\n"), 100000)
	ObserveRealtimeFrame(WithImageErrorEvidence(context.Background(), capture), "realtime_north_receive", websocket.TextMessage, body, nil)
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 1)
	content, status := ReadEvidenceEventContent(events[0], false)
	require.Equal(t, "available", status)
	require.Len(t, content.Messages, 1)
	assert.True(t, content.DisplayLimited)
	assert.True(t, content.Messages[0].DisplayLimited)
	assert.EqualValues(t, len(body), content.Messages[0].OriginalBytes)
	raw, status := ReadEvidenceEventBody(events[0], false)
	require.Equal(t, "available", status)
	_, framed := parseEvidenceObject(raw)
	assert.Equal(t, body, framed[evidenceRealtimeHeader:])
}

func TestRealtimeStagedSegmentCannotPublishAfterCleanupCommits(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ObserveRealtimeFrame(WithImageErrorEvidence(context.Background(), capture), "realtime_north_receive", websocket.TextMessage, []byte("private staged message"), nil)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 1)
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(indexes[0].Id, common.GetTimestamp()))
	// Closing and the cleanup marker race through the same publication guard.
	capture.Finish(false)
	drainImageEvidence(t)
	_, err := os.Stat(filepath.Join(env.storeDir, EvidenceObjectKey(indexes[0].Id, events[0].Id)))
	assert.True(t, os.IsNotExist(err), "late worker must not resurrect a cleaned segment")
	matches, err := filepath.Glob(filepath.Join(env.storeDir, "*", ".evidence-*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "rejected publication removes its encrypted staging file")
}

func TestRealtimeConcurrentDirectionsAndClosePreserveEveryAcceptedMessage(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ctx := WithImageErrorEvidence(context.Background(), capture)
	var wg sync.WaitGroup
	for _, stage := range []string{"realtime_north_receive", "realtime_southbound_send", "realtime_upstream_response", "realtime_client_delivery"} {
		wg.Add(1)
		go func(stage string) {
			defer wg.Done()
			for _, body := range []string{"first\nsecond", "{}{}", "last"} {
				ObserveRealtimeFrame(ctx, stage, websocket.TextMessage, []byte(body), nil)
			}
		}(stage)
	}
	wg.Wait()
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 4)
	for _, event := range events {
		content, status := ReadEvidenceEventContent(event, false)
		require.Equal(t, "available", status)
		require.Len(t, content.Messages, 3)
		for i, expected := range []string{"first\nsecond", "{}{}", "last"} {
			assert.Equal(t, expected, *content.Messages[i].Text)
		}
	}
}

func TestRealtimeIdleSegmentBecomesReadableAtFifteenSecondBoundary(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ObserveRealtimeFrame(WithImageErrorEvidence(context.Background(), capture), "realtime_north_receive", websocket.TextMessage, []byte("message"), nil)
	record := capture.realtimeRecord("realtime_north_receive")
	piece := record.piece
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 1)
	record.rotate(piece, piece.meta.CreatedAt+14)
	drainImageEvidence(t)
	_, status := ReadEvidenceEventContent(events[0], false)
	assert.Equal(t, "missing", status, "active segment is private until sealed")
	record.rotate(piece, piece.meta.CreatedAt+15)
	drainImageEvidence(t)
	content, status := ReadEvidenceEventContent(events[0], false)
	require.Equal(t, "available", status)
	require.Len(t, content.Messages, 1)
	assert.Equal(t, "message", *content.Messages[0].Text)
	capture.Finish(false)
	assert.Len(t, evidenceEventRows(t, indexes[0].Id), 1, "closing an idle sealed connection adds no empty segment")
}

func TestRealtimeRotationReusesBudgetWhilePreviousSegmentIsPublishing(t *testing.T) {
	setupEvidenceTestEnv(t)
	// Other active streams leave exactly one direction's budget available.
	for i := 0; i < cap(imageEvidenceSlots)-1; i++ {
		imageEvidenceSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < cap(imageEvidenceSlots)-1; i++ {
			<-imageEvidenceSlots
		}
	})
	before := imageEvidenceDropped.Load() + imageEvidenceFailed.Load()
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ctx := WithImageErrorEvidence(context.Background(), capture)
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, []byte("first segment"), nil)
	drainImageEvidence(t)
	record := capture.realtimeRecord("realtime_north_receive")
	entered, resume := make(chan struct{}), make(chan struct{})
	var gate sync.Once
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(resume) }) })
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("test:realtime-publish", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "TaskRequestEvidence" {
			gate.Do(func() { close(entered); <-resume })
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Query().Remove("test:realtime-publish") })
	record.rotate(record.piece, record.piece.sealAt)
	<-entered
	// The old segment still owns its slot and holds the publication guard.
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, []byte("next segment"), nil)
	capture.Finish(false)
	release.Do(func() { close(resume) })
	drainImageEvidence(t)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 2)
	for i, event := range events {
		content, status := ReadEvidenceEventContent(event, false)
		require.Equal(t, "available", status)
		require.Len(t, content.Messages, 1)
		expected := []string{"first segment", "next segment"}[i]
		assert.Equal(t, expected, *content.Messages[0].Text)
	}
	assert.Equal(t, before, imageEvidenceDropped.Load()+imageEvidenceFailed.Load(), "rotation must not require a second slot for the same direction")
}
