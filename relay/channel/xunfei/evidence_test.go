package xunfei

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestXunfeiEvidenceMatchesActualRequestAndExcludesBinaryResponse(t *testing.T) {
	previousDB, previousConfig := model.DB, system_setting.GetTaskRequestEvidenceConfig()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.EvidenceTaskLink{}))
	model.DB = db
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, RetentionDays: 2, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32)}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		require.NoError(t, service.FlushRequestEvidence(cleanupCtx))
		model.DB = previousDB
		system_setting.SetTaskRequestEvidenceConfig(previousConfig)
		require.NoError(t, service.InitTaskRequestEvidenceStore(previousConfig))
	})
	actualRequest := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, upgradeErr := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if !assert.NoError(t, upgradeErr) {
			return
		}
		defer conn.Close()
		kind, body, readErr := conn.ReadMessage()
		assert.NoError(t, readErr)
		assert.Equal(t, websocket.TextMessage, kind)
		actualRequest <- body
		assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte(`{"header":{"code":0},"payload":{"choices":{"status":2,"text":[]}}}`)))
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	capture := service.NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: "xunfei-review"})
	c.Request = c.Request.WithContext(service.WithImageErrorEvidence(ctx, capture))
	data, stop, err := xunfeiMakeRequest(c, dto.GeneralOpenAIRequest{Model: "general"}, "general", "ws"+strings.TrimPrefix(server.URL, "http"), "fixture")
	require.NoError(t, err)
	select {
	case response := <-data:
		assert.Equal(t, 2, response.Payload.Choices.Status)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-stop:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wire := <-actualRequest
	assert.True(t, bytes.HasSuffix(wire, []byte{'\n'}), "preserve the original WriteJSON framing")
	capture.Finish(false)
	require.NoError(t, service.FlushRequestEvidence(ctx))
	indexes, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "xunfei-review"})
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	sent, binary := 0, 0
	for _, event := range events {
		switch event.Stage {
		case "realtime_southbound_send":
			content, status := service.ReadEvidenceEventContent(event, false)
			require.Equal(t, "available", status)
			require.Len(t, content.Messages, 1)
			require.NotNil(t, content.Messages[0].Text)
			assert.Equal(t, string(wire), *content.Messages[0].Text)
			sent++
		case "realtime_upstream_response":
			content, status := service.ReadEvidenceEventContent(event, false)
			require.Equal(t, "available", status)
			require.Len(t, content.Messages, 1)
			assert.Equal(t, websocket.BinaryMessage, content.Messages[0].MessageType)
			assert.Nil(t, content.Messages[0].Text)
			binary++
		}
	}
	assert.Equal(t, 1, sent)
	assert.Equal(t, 1, binary)
}
