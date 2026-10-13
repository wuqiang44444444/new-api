package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIRequestEvidenceRecordsSuccessRejectionAndRetries(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		status     int
		reject     bool
	}{
		{"chat success", "/v1/chat/completions", 200, false},
		{"responses success", "/v1/responses", 200, false},
		{"embedding success", "/v1/embeddings", 200, false},
		{"rerank rejection", "/v1/rerank", 400, true},
		{"auth rejection", "/v1/moderations", 401, true},
		{"gemini success", "/v1beta/models/customer:generateContent", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"model":"provider","max_tokens":0}`, string(body))
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/retry" {
					w.WriteHeader(429)
					_, _ = w.Write([]byte(`{"error":"busy"}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":9007199254740993,"api_key":"private-provider","ok":true}`))
			}))
			defer upstream.Close()
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, "request-fixture"); c.Next() }, APIRequestEvidence())
			engine.POST(tc.path, func(c *gin.Context) {
				if tc.reject {
					_, _ = io.ReadAll(c.Request.Body)
					c.JSON(tc.status, gin.H{"error": "rejected"})
					return
				}
				c.Set("id", 7)
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"model":"customer","api_key":"private-client"}`, string(body))
				client := &http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}
				for _, path := range []string{"/retry", "/success"} {
					req, err := http.NewRequestWithContext(c.Request.Context(), "POST", upstream.URL+path, strings.NewReader(`{"model":"provider","max_tokens":0}`))
					require.NoError(t, err)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer private-provider")
					resp, err := client.Do(req)
					require.NoError(t, err)
					data, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					if resp.StatusCode == 200 {
						c.Data(tc.status, "application/json", data)
					}
				}
			})
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(`{"model":"customer","api_key":"private-client"}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, tc.status, recorder.Code)
			indices, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "request-fixture", Num: 20})
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			events := evidenceEventRows(t, indices[0].Id)
			if tc.reject {
				require.Len(t, events, 2)
				assert.Equal(t, 0, indices[0].UserID)
			} else {
				require.Len(t, events, 6)
				assert.Equal(t, 429, events[2].StatusCode)
				assert.Equal(t, 200, events[4].StatusCode)
			}
			for _, event := range events {
				payload, status := ReadEvidenceEventBody(event, false)
				require.Equal(t, "available", status)
				assert.NotContains(t, string(payload), "private-client")
				assert.NotContains(t, string(payload), "private-provider")
			}
			assert.Equal(t, tc.status, events[len(events)-1].StatusCode)
		})
	}
}

func TestAPIRequestEvidencePreservesStreamAndExcludesAdministrativeReads(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, c.Request.URL.Path); c.Next() }, APIRequestEvidence())
	stream := "data: {\"text\":\"hello\"}\n\ndata: {\"error\":\"failure\"}\n\n"
	engine.GET("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString(stream)
		c.Writer.Flush()
	})
	engine.GET("/api/task_request_evidence", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest("GET", "/v1/responses", nil))
	assert.Equal(t, 200, recorder.Code)
	assert.Equal(t, stream, recorder.Body.String())
	assert.True(t, recorder.Flushed)
	indices, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "/v1/responses", Num: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	assert.Equal(t, 200, events[1].StatusCode)
	body, status := ReadEvidenceEventBody(events[1], false)
	require.Equal(t, "available", status)
	assert.Equal(t, stream, string(body))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/task_request_evidence", nil))
	_, count, err = model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "/api/task_request_evidence", Num: 20})
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestBackgroundRequestEvidenceKeepsTaskIdentity(t *testing.T) {
	setupEvidenceTestEnv(t)
	task := &model.Task{TaskID: "task-background", UserId: 7, ChannelId: 9, Properties: model.Properties{OriginModelName: "customer"}}
	ctx, finish := BeginBackgroundRequestEvidence(context.Background(), task)
	capture := apiEvidenceFrom(ctx)
	require.NotNil(t, capture)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.example/tasks/1", nil)
	require.NoError(t, err)
	capture.requestRecord(req, model.TaskRequestEvidenceStageSouthboundSend)
	finish()
	indices, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID, Num: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, 7, indices[0].UserID)
	assert.Equal(t, 9, indices[0].ChannelID)
	assert.Equal(t, "customer", indices[0].ClientModel)
}

func TestAPIRequestEvidencePreservesMalformedAndTruncatedRequests(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
	}{
		{"malformed", `{"model":"customer","api_key":"private-client","keep":9007199254740993`, 1024},
		{"truncated", `{"model":"customer","prompt":"long prompt"}`, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			config := system_setting.GetTaskRequestEvidenceConfig()
			config.MaxBodyBytes = tc.limit
			system_setting.SetTaskRequestEvidenceConfig(config)
			engine := gin.New()
			engine.Use(APIRequestEvidence())
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(body))
				c.String(400, "invalid request")
			})
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, 400, recorder.Code)
			indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
			require.NoError(t, err)
			require.Len(t, indices, 1)
			events := evidenceEventRows(t, indices[0].Id)
			require.Len(t, events, 2)
			payload, status := ReadEvidenceEventBody(events[0], false)
			assert.Equal(t, "available", status)
			assert.NotContains(t, string(payload), "private-client")
			assert.EqualValues(t, len(tc.body), events[0].ByteCount)
			if tc.name == "truncated" {
				assert.False(t, events[0].Complete)
				assert.Equal(t, model.TaskRequestEvidencePhaseTruncated, events[0].Phase)
			} else {
				assert.Contains(t, string(payload), "9007199254740993")
			}
			previews := GetEvidenceEventPreviews(events, false)
			assert.NotEmpty(t, previews[events[0].Id].Text)
		})
	}
}

func TestAPIRequestEvidenceStorageFailureDoesNotChangeClientResponse(t *testing.T) {
	setupEvidenceTestEnv(t)
	original := GetTaskRequestEvidenceStore()
	evidenceObjectStore = nil
	t.Cleanup(func() { evidenceObjectStore = original })
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/embeddings", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"model":"customer"}`)))
	assert.Equal(t, 200, recorder.Code)
	assert.JSONEq(t, `{"ok":true}`, recorder.Body.String())
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	for _, event := range events {
		assert.Equal(t, model.TaskRequestEvidencePhaseUnavailable, event.Phase)
		assert.Contains(t, event.Detail, "capture_failure")
	}
}

func TestAPIObservedBodyRedactsDamagedCredentials(t *testing.T) {
	for _, body := range []string{`{"api_key":"private`, `{"password":{"secret":"private"},"keep":1`, "data: {\"api_key\":\"private\",\"keep\":1\ndata: {\"broken\":", `{"api_key":false,"keep":1`} {
		payload, err := redactAPIObservedBody([]byte(body), "application/json")
		require.NoError(t, err)
		assert.NotContains(t, string(payload), "private")
		assert.Contains(t, string(payload), "[REDACTED]")
	}
}

func TestAPIRequestEvidencePreservesRequestReadError(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		assert.Equal(t, "prefix", string(body))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		c.String(400, "read failed")
	})
	reader := io.MultiReader(strings.NewReader("prefix"), &apiEvidenceReadError{err: io.ErrUnexpectedEOF})
	req := httptest.NewRequest("POST", "/v1/chat/completions", reader)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	assert.Equal(t, 400, recorder.Code)
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	assert.False(t, events[0].Complete)
	assert.Contains(t, events[0].Detail, `"read_failed":true`)
}

func TestAPIRequestEvidenceRecordsActualRealtimeHandshakeAndFrames(t *testing.T) {
	setupEvidenceTestEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		_, message, err := conn.ReadMessage()
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, message))
	}))
	defer upstream.Close()
	done := make(chan struct{})
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Next(); close(done) }, APIRequestEvidence())
	engine.GET("/v1/realtime", func(c *gin.Context) {
		client, err := (&websocket.Upgrader{}).Upgrade(c.Writer, c.Request, nil)
		require.NoError(t, err)
		defer client.Close()
		target, err := DialRealtimeWithEvidence(c, "ws"+strings.TrimPrefix(upstream.URL, "http"), nil)
		require.NoError(t, err)
		defer target.Close()
		_, message, err := client.ReadMessage()
		require.NoError(t, err)
		ObserveRealtimeFrame(c.Request.Context(), "realtime_north_receive", message, nil)
		require.NoError(t, target.WriteMessage(websocket.TextMessage, message))
		ObserveRealtimeFrame(c.Request.Context(), "realtime_southbound_send", message, nil)
		_, result, err := target.ReadMessage()
		require.NoError(t, err)
		ObserveRealtimeFrame(c.Request.Context(), "realtime_upstream_response", result, nil)
		require.NoError(t, client.WriteMessage(websocket.TextMessage, result))
		ObserveRealtimeFrame(c.Request.Context(), "realtime_client_delivery", result, nil)
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", nil)
	require.NoError(t, err)
	defer client.Close()
	assert.Equal(t, 101, resp.StatusCode)
	message := []byte(`{"type":"event","api_key":"private-key","value":9007199254740993}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, message))
	_, result, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, message, result)
	<-done
	server.Close()
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	var handshake, delivery bool
	for _, event := range events {
		if event.Stage == model.TaskRequestEvidenceStageUpstreamResponse {
			assert.Equal(t, 101, event.StatusCode)
			handshake = true
		}
		if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
			assert.Equal(t, 101, event.StatusCode)
			delivery = true
		}
		if strings.HasPrefix(event.Stage, "realtime_") {
			payload, status := ReadEvidenceEventBody(event, false)
			assert.Equal(t, "available", status)
			assert.NotContains(t, string(payload), "private-key")
			assert.Contains(t, string(payload), "9007199254740993")
		}
	}
	assert.True(t, handshake)
	assert.True(t, delivery)
}

func TestAPIObservedBodyProtectsEscapedSourceAndSignedURLs(t *testing.T) {
	capture := &ImageErrorEvidence{}
	payload, err := capture.redactObservedBody([]byte(`{"url":"https:\/\/cdn.example\/image?X-Amz-Signature=private"}`), "application/json")
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "private")
	capture.suppressSourceURLs = true
	payload, err = capture.redactObservedBody([]byte(`{"url":"https:\/\/cdn.example\/image","model":"customer"}`), "application/json")
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "cdn.example")
	assert.Contains(t, string(payload), "customer")
}

func TestAPIRequestEvidenceIndependentOfLegacyCapacity(t *testing.T) {
	setupEvidenceTestEnv(t)
	reserved := 0
	for i := 0; i < cap(imageEvidenceSlots); i++ {
		select {
		case imageEvidenceSlots <- struct{}{}:
			reserved++
		default:
			t.Fatal("fixture requires an idle capture budget")
		}
	}
	defer func() {
		for i := 0; i < reserved; i++ {
			<-imageEvidenceSlots
		}
	}()
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/embeddings", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"input":"sample"}`, string(body))
		c.JSON(200, gin.H{"ok": true})
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"input":"sample"}`)))
	assert.Equal(t, 200, response.Code)
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	assert.EqualValues(t, len(`{"input":"sample"}`), events[0].ByteCount)
	assert.Equal(t, 200, events[1].StatusCode)
	for _, event := range events {
		assert.True(t, event.Complete)
		assert.NotContains(t, event.Detail, "capture_capacity_unavailable")
	}
}

func TestAPIRequestEvidencePanicRecordsFinalDelivery(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "panic before headers", true: "panic after SSE"}[streaming], func(t *testing.T) {
			setupEvidenceTestEnv(t)
			engine := gin.New()
			engine.Use(APIRequestEvidence(), gin.CustomRecovery(func(c *gin.Context, _ any) { c.JSON(500, gin.H{"error": "panic"}) }))
			engine.GET("/v1/chat/completions", func(c *gin.Context) {
				if streaming {
					c.Writer.Header().Set("Content-Type", "text/event-stream")
					_, _ = c.Writer.WriteString("data: {\"ok\":true}\n\n")
					c.Writer.Flush()
				}
				panic("synthetic")
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/chat/completions", nil))
			expected := 500
			if streaming {
				expected = 200
			}
			assert.Equal(t, expected, response.Code)
			indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
			require.NoError(t, err)
			require.Len(t, indices, 1)
			events := evidenceEventRows(t, indices[0].Id)
			require.Len(t, events, 2)
			assert.Equal(t, response.Code, events[1].StatusCode)
			payload, status := ReadEvidenceEventBody(events[1], false)
			assert.Equal(t, "available", status)
			assert.Contains(t, string(payload), "panic")
		})
	}
}

func TestAPIRequestEvidenceRejectsBeforeReadingBody(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/embeddings", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	req := httptest.NewRequest("POST", "/v1/embeddings", nil)
	req.Body = io.NopCloser(&apiNeverReadBody{t: t})
	req.ContentLength = 100
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, req)
	assert.Equal(t, 401, response.Code)
}

type apiNeverReadBody struct{ t *testing.T }

func (b *apiNeverReadBody) Read([]byte) (int, error) {
	b.t.Fatal("authentication must run without reading a slow body")
	return 0, io.EOF
}

func TestAPIRequestEvidenceRejectedBodyOnRealHTTP(t *testing.T) {
	setupEvidenceTestEnv(t)
	done := make(chan struct{})
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Next(); close(done) }, APIRequestEvidence())
	engine.POST("/v1/embeddings", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	server := httptest.NewServer(APIRequestEvidenceHTTPHandler(engine))
	defer server.Close()
	resp, err := http.Post(server.URL+"/v1/embeddings", "application/json", strings.NewReader(`{"input":"sample","api_key":"private"}`))
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	<-done
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	payload, status := ReadEvidenceEventBody(events[0], false)
	assert.Equal(t, "available", status)
	assert.Contains(t, string(payload), "sample")
	assert.NotContains(t, string(payload), "private")
	assert.True(t, events[0].Complete)
}

func TestEvidenceMetadataRemovesSignedURLsBeforeStorage(t *testing.T) {
	headers := EvidenceRedactHeaders(http.Header{"Location": {"https://cdn.example/file?X-Amz-Signature=private"}})
	assert.NotContains(t, headers["Location"], "private")
	query := EvidenceRedactQueryParams(url.Values{"url": {"https://cdn.example/file?sig=private"}, "model": {"customer"}})
	assert.NotContains(t, query["url"], "private")
	assert.Equal(t, "customer", query["model"])
	capture := &ImageErrorEvidence{secrets: []string{"1"}}
	payload, err := capture.redactObservedBody([]byte(`{"n":1,"max_tokens":100,"echo":"1","api_key":"1"}`), "application/json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"n":1,"max_tokens":100,"echo":"[REDACTED]","api_key":"[REDACTED]"}`, string(payload))
}

func TestTaskPollingEvidenceRetainsPOSTAndOneResponsePerTask(t *testing.T) {
	setupEvidenceTestEnv(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, `{"ids":["a","b"]}`, string(body))
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(503)
		_, _ = w.Write([]byte("provider unavailable"))
	}))
	defer provider.Close()
	tasks := []*model.Task{{TaskID: "task-a", UserId: 7}, {TaskID: "task-b", UserId: 7}}
	req, err := http.NewRequest("POST", provider.URL, strings.NewReader(`{"ids":["a","b"]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := DoTaskPollingRequestWithEvidence(provider.Client(), req, tasks)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	for _, task := range tasks {
		CaptureFallbackTaskPollingHTTPResponse(apiRawPollingFixture{}, task, provider.URL, resp, body, nil)
		indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID, Num: 20})
		require.NoError(t, err)
		require.Len(t, indices, 1)
		events := evidenceEventRows(t, indices[0].Id)
		require.Len(t, events, 2)
		assert.Equal(t, http.MethodPost, events[0].Method)
		payload, status := ReadEvidenceEventBody(events[0], false)
		assert.Equal(t, "available", status)
		assert.JSONEq(t, `{"ids":["a","b"]}`, string(payload))
		assert.Equal(t, 503, events[1].StatusCode)
		assert.Equal(t, "text/plain", events[1].ContentType)
	}
}

type apiRawPollingFixture struct{}

func (apiRawPollingFixture) CapturesRawPollingEvidence() {}

type apiEvidenceReadError struct{ err error }

func (r *apiEvidenceReadError) Read(_ []byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	err := r.err
	r.err = nil
	return 0, err
}

func TestAPIRequestEvidenceNineConcurrentRequestsKeepBodies(t *testing.T) {
	setupEvidenceTestEnv(t)
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	finished := make(chan struct{}, 9)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, c.Param("id")); c.Next(); finished <- struct{}{} }, APIRequestEvidence())
	engine.POST("/v1/embeddings/:id", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"input":"sample"}`, string(body))
		if c.Param("id") != "9" {
			started <- struct{}{}
			<-release
		}
		c.JSON(200, gin.H{"ok": true})
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	results := make(chan error, 8)
	for i := 1; i <= 8; i++ {
		go func(id int) {
			resp, err := http.Post(fmt.Sprintf("%s/v1/embeddings/%d", server.URL, id), "application/json", strings.NewReader(`{"input":"sample"}`))
			if err == nil {
				_, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
			results <- err
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-started
	}
	resp, err := http.Post(server.URL+"/v1/embeddings/9", "application/json", strings.NewReader(`{"input":"sample"}`))
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	close(release)
	for i := 0; i < 8; i++ {
		require.NoError(t, <-results)
	}
	for i := 0; i < 9; i++ {
		<-finished
	}
	indices, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.EqualValues(t, 9, count)
	for _, index := range indices {
		events := evidenceEventRows(t, index.Id)
		require.Len(t, events, 2)
		payload, status := ReadEvidenceEventBody(events[0], false)
		assert.Equal(t, "available", status)
		assert.JSONEq(t, `{"input":"sample"}`, string(payload))
		assert.True(t, events[0].Complete)
		payload, status = ReadEvidenceEventBody(events[1], false)
		assert.Equal(t, "available", status)
		assert.JSONEq(t, `{"ok":true}`, string(payload))
		assert.True(t, events[1].Complete)
	}
}

func TestAPIRequestEvidenceSlowRejectedBodyEndsAtReadDeadline(t *testing.T) {
	setupEvidenceTestEnv(t)
	done := make(chan struct{})
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Next(); close(done) }, APIRequestEvidence())
	engine.POST("/v1/embeddings", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	server := httptest.NewServer(APIRequestEvidenceHTTPHandler(engine))
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	defer conn.Close()
	_, err = io.WriteString(conn, "POST /v1/embeddings HTTP/1.1\r\nHost: fixture\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	<-done
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	assert.EqualValues(t, 1, events[0].ByteCount)
	assert.False(t, events[0].Complete)
	assert.Contains(t, events[0].Detail, `"read_failed":true`)
	assert.Equal(t, 401, events[1].StatusCode)
}

func TestAPIRequestEvidenceCredentialEchoKeepsNumericValues(t *testing.T) {
	capture := &ImageErrorEvidence{secrets: []string{"1", "synthetic-long-secret"}}
	payload, err := capture.redactObservedBody([]byte(`{"n":1,"message":"echo synthetic-long-secret"}`), "application/json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"n":1,"message":"echo [REDACTED]"}`, string(payload))
	payload, err = capture.redactObservedBody([]byte("echo synthetic-long-secret"), "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "echo [REDACTED]", string(payload))
}
