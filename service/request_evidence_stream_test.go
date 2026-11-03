package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvidenceLargeOpaqueBodyExceedsOldLimitsWithoutTruncation(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "api_request"})
	record := capture.newRecord("north_receive", EvidenceObjectMeta{ContentType: "application/json"}, false)
	// Real >32 MiB contract: bounded source block, no full test payload or Get.
	block := bytes.Repeat([]byte("invalid JSON, preserve bytes\x00\xff\n"), 2200)[:64<<10]
	expected := sha256.New()
	const blocks = 529
	for i := 0; i < blocks; i++ {
		record.append(block)
		_, _ = expected.Write(block)
	}
	record.close()
	capture.Finish(false)
	drainImageEvidence(t)
	items := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, items, 1)
	events := evidenceEventRows(t, items[0].Id)
	require.Len(t, events, 1)
	reader, status := OpenEvidenceEventBody(events[0], false)
	require.Equal(t, "available", status)
	defer reader.Close()
	raw := bufio.NewReader(reader)
	_, err := raw.ReadString('\n')
	require.NoError(t, err)
	_, err = raw.ReadString('\n')
	require.NoError(t, err)
	actual := sha256.New()
	n, err := io.CopyBuffer(actual, raw, make([]byte, 64<<10))
	require.NoError(t, err)
	assert.EqualValues(t, blocks*len(block), n)
	assert.Equal(t, expected.Sum(nil), actual.Sum(nil))
	view, status := ReadEvidenceEventContent(events[0], false)
	require.Equal(t, "available", status)
	assert.True(t, view.DisplayLimited)
	assert.Len(t, view.Body, 64<<10)
	assert.Equal(t, block, view.Body)
}

func TestEvidenceStreamAuthenticatesIdentityOrderAndFinalPackage(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	payload := bytes.Repeat([]byte("private fixture bytes"), 10000)
	require.NoError(t, store.Put("7/8.bin", payload))
	original, err := os.ReadFile(filepath.Join(store.baseDir, "7/8.bin"))
	require.NoError(t, err)
	assert.NotContains(t, string(original), "private fixture bytes")
	require.NoError(t, os.WriteFile(filepath.Join(store.baseDir, "7/9.bin"), original, 0600))
	_, err = store.Get("7/9.bin")
	require.Error(t, err, "ciphertext is bound to its event path")
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"tampered", func(b []byte) []byte { b[len(b)/2] ^= 1; return b }},
		{"truncated", func(b []byte) []byte { return b[:len(b)-1] }},
		{"reordered", func(b []byte) []byte {
			const packet = (64 << 10) + 32
			offset := len(evidenceStreamMagic)
			a := append([]byte(nil), b[offset:offset+packet]...)
			copy(b[offset:offset+packet], b[offset+packet:offset+2*packet])
			copy(b[offset+packet:offset+2*packet], a)
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(store.baseDir, "7/8.bin"), tc.mutate(append([]byte(nil), original...)), 0600))
			_, err := store.Get("7/8.bin")
			require.Error(t, err)
		})
	}
}

func TestEvidenceRealtimeMessagesPreserveBytesAndExcludeBinary(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
	ctx := context.WithValue(context.Background(), apiRequestEvidenceKey{}, capture)
	text := []byte(" {\"type\":\"audio\",\"unknown\":0}\n")
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, text, nil)
	ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.BinaryMessage, []byte("file bytes"), nil)
	capture.Finish(false)
	items := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, items, 1)
	events := evidenceEventRows(t, items[0].Id)
	require.Len(t, events, 1, "one segment per direction, not per message")
	content, status := ReadEvidenceEventContent(events[0], false)
	require.Equal(t, "available", status)
	require.Len(t, content.Messages, 2)
	require.NotNil(t, content.Messages[0].Text)
	assert.Equal(t, string(text), *content.Messages[0].Text)
	assert.Equal(t, websocket.TextMessage, content.Messages[0].MessageType)
	assert.Nil(t, content.Messages[1].Text)
	assert.Empty(t, content.Messages[1].Base64)
	assert.EqualValues(t, len("file bytes"), content.Messages[1].OriginalBytes)
	raw, status := ReadEvidenceEventBody(events[0], false)
	require.Equal(t, "available", status)
	assert.NotContains(t, string(raw), "file bytes")
}

func TestEvidenceSaturatedObjectBudgetLeavesBusinessBytesIntact(t *testing.T) {
	setupEvidenceTestEnv(t)
	for i := 0; i < cap(imageEvidenceSlots); i++ {
		imageEvidenceSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(imageEvidenceSlots); i++ {
			<-imageEvidenceSlots
		}
	}()
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{})
	original := []byte("unmodified business response")
	req, _ := http.NewRequest("POST", "https://provider.invalid", nil)
	record := capture.requestRecord(req, "north_receive")
	body := &imageEvidenceBody{ReadCloser: io.NopCloser(bytes.NewReader(original)), capture: capture, record: record}
	actual, err := io.ReadAll(body)
	require.NoError(t, err)
	require.NoError(t, body.Close())
	capture.Finish(false)
	assert.Equal(t, original, actual)
	drainImageEvidence(t)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidence{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEvidenceBatchPollingStoresOneExchangeForKnownTasks(t *testing.T) {
	setupEvidenceTestEnv(t)
	tasks := []*model.Task{{TaskID: "one", UserId: 7, AppID: 3}, {TaskID: "two", UserId: 7, AppID: 3}}
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "task_polling"})
	for _, task := range tasks {
		capture.tasks = append(capture.tasks, model.EvidenceTaskLink{TaskID: task.TaskID, UserID: task.UserId, AppID: task.AppID})
	}
	record := capture.newRecord("polling", EvidenceObjectMeta{}, false)
	record.append([]byte("shared response"))
	record.close()
	capture.Finish(false)
	first := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{TaskID: "one"})
	second := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{TaskID: "two"})
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, first[0].Id, second[0].Id)
	events := evidenceEventRows(t, first[0].Id)
	require.Len(t, events, 1)
	assert.Equal(t, "shared response", string(evidenceCapturedBody(t, events[0])))
}

func TestEvidenceDatabaseWriteCannotDelayBusinessResponse(t *testing.T) {
	setupEvidenceTestEnv(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("test:evidence-storage-gate", func(tx *gorm.DB) {
		if tx.Statement.Table == "task_request_evidences" {
			once.Do(func() { close(entered) })
			<-release
		}
	}))
	defer model.DB.Callback().Create().Remove("test:evidence-storage-gate")
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/responses", func(c *gin.Context) { c.String(200, "business completed") })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("POST", "/v1/responses", nil))
	assert.Equal(t, 200, response.Code)
	assert.Equal(t, "business completed", response.Body.String())
	<-entered
	close(release)
	drainImageEvidence(t)
}

func TestEvidenceStorageFailurePreservesActualProviderExchange(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("fixture"), 0600))
	store.baseDir = file
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "opaque request", string(body))
		w.WriteHeader(429)
		_, _ = w.Write([]byte("upstream rejection"))
	}))
	defer upstream.Close()
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/responses", func(c *gin.Context) {
		request, err := http.NewRequestWithContext(c.Request.Context(), "POST", upstream.URL, c.Request.Body)
		require.NoError(t, err)
		client := &http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		c.Status(response.StatusCode)
		_, err = io.Copy(c.Writer, response.Body)
		require.NoError(t, err)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("POST", "/v1/responses", strings.NewReader("opaque request")))
	drainImageEvidence(t)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 429, response.Code)
	assert.Equal(t, "upstream rejection", response.Body.String())
}

func TestEvidenceRotationPreservesIndependentCompletePieces(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{})
	record := capture.newRecord("client_delivery", EvidenceObjectMeta{}, false)
	record.append([]byte("before rotation"))
	record.rotate(record.piece, record.piece.sealAt)
	// The worker's hourly boundary seals even an idle connection's current
	// piece. The next observed bytes enter a separate immutable object.
	drainImageEvidence(t)
	record.append([]byte("after rotation"))
	record.close()
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 2)
	assert.Equal(t, "before rotation", string(evidenceCapturedBody(t, events[0])))
	assert.Equal(t, "after rotation", string(evidenceCapturedBody(t, events[1])))
}

func TestEvidenceLegacyCiphertextRemainsReadableWithoutRewrite(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	payload := []byte("historical opaque body")
	nonce := bytes.Repeat([]byte{3}, store.aead.NonceSize())
	sealed := store.aead.Seal(append([]byte(nil), nonce...), nonce, payload, nil)
	path := filepath.Join(store.baseDir, "legacy.bin")
	require.NoError(t, os.WriteFile(path, sealed, 0600))
	reader, status := OpenEvidenceEventBody(&model.TaskRequestEvidenceEvent{Id: 1, ObjectKey: "legacy.bin"}, false)
	require.Equal(t, "available", status)
	defer reader.Close()
	actual, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, payload, actual)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, sealed, after)
}
