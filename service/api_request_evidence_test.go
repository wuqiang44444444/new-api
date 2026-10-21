package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
		{"chat success", "/v1/images/edits", 200, false},
		{"audio success", "/v1/audio/translations", 200, false},
		{"embedding success", "/v1/images/generations", 200, false},
		{"rerank rejection", "/v1/audio/transcriptions", 400, true},
		{"auth rejection", "/v1/audio/speech", 401, true},
		{"gemini success", "/api/v3/contents/generations/tasks", 200, false},
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
			var captured strings.Builder
			for _, event := range events {
				payload, status := ReadEvidenceEventBody(event, false)
				require.Equal(t, "available", status)
				assert.Empty(t, event.Phase, "raw evidence keeps no phase state")
				assert.Empty(t, event.Detail)
				captured.Write(payload)
			}
			// Raw evidence keeps captured bytes as-is, credentials included; masking happens on admin view.
			assert.Contains(t, captured.String(), "private-client")
			if !tc.reject {
				assert.Contains(t, captured.String(), "private-provider")
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
	engine.GET("/v1/realtime", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString(stream)
		c.Writer.Flush()
	})
	engine.GET("/api/task_request_evidence", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest("GET", "/v1/realtime", nil))
	assert.Equal(t, 200, recorder.Code)
	assert.Equal(t, stream, recorder.Body.String())
	assert.True(t, recorder.Flushed)
	indices, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "/v1/realtime", Num: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	assert.Equal(t, 200, events[1].StatusCode)
	body, status := ReadEvidenceEventBody(events[1], false)
	require.Equal(t, "available", status)
	_, raw := parseEvidenceObject(body)
	assert.Equal(t, stream, string(raw))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/task_request_evidence", nil))
	_, count, err = model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "/api/task_request_evidence", Num: 20})
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestBackgroundRequestEvidenceKeepsTaskIdentity(t *testing.T) {
	setupEvidenceTestEnv(t)
	task := &model.Task{TaskID: "task-background", UserId: 7, ChannelId: 9, ClientProtocol: model.TaskClientProtocolImageOpenAIV1, Properties: model.Properties{OriginModelName: "customer"}}
	require.NoError(t, model.DB.Create(task).Error)
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
			engine.POST("/v1/images/edits", func(c *gin.Context) {
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(body))
				c.String(400, "invalid request")
			})
			req := httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(tc.body))
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
			_, raw := parseEvidenceObject(payload)
			// Malformed and capture-limited bytes stay exactly as observed, credentials included.
			assert.NotEmpty(t, string(raw))
			assert.Equal(t, tc.body[:min(int64(len(tc.body)), system_setting.GetTaskRequestEvidenceConfig().MaxBodyBytes, system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes)], string(raw))
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
	engine.POST("/v1/images/generations", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"model":"customer"}`)))
	assert.Equal(t, 200, recorder.Code)
	assert.JSONEq(t, `{"ok":true}`, recorder.Body.String())
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	for _, event := range events {
		_, status := ReadEvidenceEventBody(event, false)
		assert.Equal(t, "storage_unavailable", status)
	}
}

func TestAPIRequestEvidencePreservesRequestReadError(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/images/edits", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		assert.Equal(t, "prefix", string(body))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		c.String(400, "read failed")
	})
	reader := io.MultiReader(strings.NewReader("prefix"), &apiEvidenceReadError{err: io.ErrUnexpectedEOF})
	req := httptest.NewRequest("POST", "/v1/images/edits", reader)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	assert.Equal(t, 400, recorder.Code)
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	payload, status := ReadEvidenceEventBody(events[0], false)
	assert.Equal(t, "available", status)
	meta, raw := parseEvidenceObject(payload)
	require.NotNil(t, meta, "v2 object header expected")
	assert.Equal(t, io.ErrUnexpectedEOF.Error(), meta.Error)
	assert.Equal(t, "prefix", string(raw))
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
			assert.Contains(t, string(payload), "private-key")
			assert.Contains(t, string(payload), "9007199254740993")
		}
	}
	assert.True(t, handshake)
	assert.True(t, delivery)
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
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"input":"sample"}`, string(body))
		c.JSON(200, gin.H{"ok": true})
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"input":"sample"}`)))
	assert.Equal(t, 200, response.Code)
	indices, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 20})
	require.NoError(t, err)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 2)
	payload, status := ReadEvidenceEventBody(events[0], false)
	require.Equal(t, "available", status)
	_, raw := parseEvidenceObject(payload)
	assert.Equal(t, `{"input":"sample"}`, string(raw))
	assert.Equal(t, 200, events[1].StatusCode)
}

func TestAPIRequestEvidencePanicRecordsFinalDelivery(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "panic before headers", true: "panic after SSE"}[streaming], func(t *testing.T) {
			setupEvidenceTestEnv(t)
			engine := gin.New()
			engine.Use(APIRequestEvidence(), gin.CustomRecovery(func(c *gin.Context, _ any) { c.JSON(500, gin.H{"error": "panic"}) }))
			engine.GET("/v1/realtime", func(c *gin.Context) {
				if streaming {
					c.Writer.Header().Set("Content-Type", "text/event-stream")
					_, _ = c.Writer.WriteString("data: {\"ok\":true}\n\n")
					c.Writer.Flush()
				}
				panic("synthetic")
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/realtime", nil))
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
	engine.POST("/v1/images/generations", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	req := httptest.NewRequest("POST", "/v1/images/generations", nil)
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
	engine.POST("/v1/images/generations", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	server := httptest.NewServer(APIRequestEvidenceHTTPHandler(engine))
	defer server.Close()
	resp, err := http.Post(server.URL+"/v1/images/generations", "application/json", strings.NewReader(`{"input":"sample","api_key":"private"}`))
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
	_, raw := parseEvidenceObject(payload)
	assert.Equal(t, `{"input":"sample","api_key":"private"}`, string(raw))
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
	tasks := []*model.Task{{TaskID: "task-a", UserId: 7, ClientProtocol: "openai_video"}, {TaskID: "task-b", UserId: 7, ClientProtocol: "openai_video"}}
	for _, task := range tasks {
		require.NoError(t, model.DB.Create(task).Error)
	}
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
		_, raw := parseEvidenceObject(payload)
		assert.JSONEq(t, `{"ids":["a","b"]}`, string(raw))
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
	engine.POST("/v1/videos/:id/remix", func(c *gin.Context) {
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
			resp, err := http.Post(fmt.Sprintf("%s/v1/videos/%d/remix", server.URL, id), "application/json", strings.NewReader(`{"input":"sample"}`))
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
	resp, err := http.Post(server.URL+"/v1/videos/9/remix", "application/json", strings.NewReader(`{"input":"sample"}`))
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
		_, requestBody := parseEvidenceObject(payload)
		assert.JSONEq(t, `{"input":"sample"}`, string(requestBody))
		payload, status = ReadEvidenceEventBody(events[1], false)
		assert.Equal(t, "available", status)
		_, responseBody := parseEvidenceObject(payload)
		assert.JSONEq(t, `{"ok":true}`, string(responseBody))
	}
}

func TestAPIRequestEvidenceSlowRejectedBodyEndsAtReadDeadline(t *testing.T) {
	setupEvidenceTestEnv(t)
	done := make(chan struct{})
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Next(); close(done) }, APIRequestEvidence())
	engine.POST("/v1/images/generations", func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"}) })
	server := httptest.NewServer(APIRequestEvidenceHTTPHandler(engine))
	defer server.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	defer conn.Close()
	_, err = io.WriteString(conn, "POST /v1/images/generations HTTP/1.1\r\nHost: fixture\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{")
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
	payload, status := ReadEvidenceEventBody(events[0], false)
	require.Equal(t, "available", status)
	meta, raw := parseEvidenceObject(payload)
	require.NotNil(t, meta, "v2 object header expected")
	assert.NotEmpty(t, meta.Error, "the read deadline must surface as the error note")
	assert.Equal(t, "{", string(raw))
	assert.Equal(t, 401, events[1].StatusCode)
}

// 全站观察器与音视频证据会话的同阶段去重：会话持有交换证据时，通用观察器
// 丢弃记录，不向同一业务请求落第二份证据；无会话时照常持久化。
func TestAPIRequestEvidenceDefersToTaskEvidenceSession(t *testing.T) {
	for _, tc := range []struct {
		name        string
		sessionKind string
	}{
		{"task session owns the exchange", model.TaskRequestEvidenceKindVideoTask},
		{"no session persists once", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, "dedup-fixture"); c.Next() }, APIRequestEvidence())
			engine.POST("/v1/video/generations", func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("token_id", 42)
				if tc.sessionKind != "" {
					require.NoError(t, BeginTaskRequestEvidence(c, tc.sessionKind))
				}
				_, _ = io.ReadAll(c.Request.Body)
				c.JSON(http.StatusOK, gin.H{"task_id": "task-dedup"})
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"seedance-1"}`))
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)

			_, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "dedup-fixture", Num: 10})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			indexes, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "dedup-fixture"})
			require.NoError(t, err)
			events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
			require.NoError(t, err)
			stages := map[string]int{}
			for _, event := range events {
				stages[event.Stage]++
			}
			assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageNorthReceive])
			assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageClientDelivery])
		})
	}
}
