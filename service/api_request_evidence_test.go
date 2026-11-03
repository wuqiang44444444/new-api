package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIRequestEvidencePreservesActualFourDirections(t *testing.T) {
	for _, status := range []int{200, 400, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			setupEvidenceTestEnv(t)
			north := []byte(" {\"model\":\"customer\", \"duplicate\":1, \"duplicate\":2,\"n\":0}\n")
			south := []byte("{\"model\":\"provider\",\"n\":0}")
			upstreamBody := []byte("data: {\"id\":9007199254740993}\n\ndata: [DONE]\n\n")
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, south, got)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(status)
				_, _ = w.Write(upstreamBody)
			}))
			defer provider.Close()
			engine := gin.New()
			engine.Use(APIRequestEvidence())
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("app_id", 3)
				got, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, north, got)
				req, err := http.NewRequestWithContext(c.Request.Context(), "POST", provider.URL, bytes.NewReader(south))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				response, err := (&http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}).Do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				c.Data(response.StatusCode, "text/event-stream", body)
			})
			req := httptest.NewRequest("POST", "/v1/chat/completions?x=%2f&x=/", bytes.NewReader(north))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, status, recorder.Code)
			assert.Equal(t, upstreamBody, recorder.Body.Bytes())
			indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
			require.Len(t, indexes, 1)
			assert.Equal(t, 7, indexes[0].UserID)
			assert.Equal(t, 3, indexes[0].AppID)
			events := evidenceEventRows(t, indexes[0].Id)
			require.Len(t, events, 4)
			bodies := map[string][]byte{}
			for _, event := range events {
				bodies[event.Stage] = evidenceCapturedBody(t, event)
			}
			assert.Equal(t, north, bodies[model.TaskRequestEvidenceStageNorthReceive])
			assert.Equal(t, south, bodies[model.TaskRequestEvidenceStageSouthboundSend])
			assert.Equal(t, upstreamBody, bodies[model.TaskRequestEvidenceStageUpstreamResponse])
			assert.Equal(t, upstreamBody, bodies[model.TaskRequestEvidenceStageClientDelivery])
			for _, event := range events {
				if event.Stage == model.TaskRequestEvidenceStageNorthReceive {
					content, s := ReadEvidenceEventContent(event, false)
					require.Equal(t, "available", s)
					assert.Equal(t, "x=%2f&x=/", content.Meta.RawQuery)
				}
			}
		})
	}
}
func TestAPIRequestEvidenceLocalRejectionDoesNotReadInput(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/responses", func(c *gin.Context) { c.JSON(401, gin.H{"error": "unauthorized"}) })
	req := httptest.NewRequest("POST", "/v1/responses", &apiNeverReadBody{t: t})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	assert.Equal(t, 401, recorder.Code)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 2)
	for _, event := range events {
		if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
			assert.Contains(t, string(evidenceCapturedBody(t, event)), "unauthorized")
		}
	}
}

type apiNeverReadBody struct{ t *testing.T }

func (b *apiNeverReadBody) Read([]byte) (int, error) {
	b.t.Fatal("evidence must not drain rejected input")
	return 0, io.EOF
}
func (b *apiNeverReadBody) Close() error { return nil }
func TestAPIRequestEvidencePreservesPanicPresentation(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence(), gin.CustomRecovery(func(c *gin.Context, _ any) { c.String(500, "final failure") }))
	engine.POST("/v1/responses", func(c *gin.Context) { panic("fixture") })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest("POST", "/v1/responses", strings.NewReader("{}")))
	assert.Equal(t, 500, recorder.Code)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	for _, event := range evidenceEventRows(t, indexes[0].Id) {
		if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
			assert.Equal(t, "final failure", string(evidenceCapturedBody(t, event)))
		}
	}
}
