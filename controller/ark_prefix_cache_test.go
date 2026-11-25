package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkCacheErrorKeepsThePublicCodeInTheRelayEvent(t *testing.T) {
	engine := gin.New()
	events := logtest.New(t)
	engine.Use(events.Middleware())
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.Set("id", 3)
		c.Set("route_tag", "relay")
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{Stage: "contract_scope", Reason: "contract_model_not_listed"})
		arkCacheError(c, http.StatusForbidden, "model_not_found")
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, events.String(), "stage=contract_scope reason=contract_model_not_listed public_code=model_not_found")
}

func TestArkPrefixCacheLeavesOrdinaryResponsesToTheExistingHandler(t *testing.T) {
	for _, body := range []string{
		`{"model":"other-provider","caching":{"type":"enabled","prefix":true},"input":"ordinary"}`,
		`{"model":"seed-lite","previous_response_id":"resp_from_another_provider","store":true}`,
		`{"model":"seed-lite","input":"ordinary seed lite request"}`,
	} {
		engine := gin.New()
		pilot := &service.ArkPrefixCachePilot{UserID: 3, TokenID: 5, Model: "seed-lite", Mode: "observe"}
		engine.POST("/v1/responses", func(c *gin.Context) {
			defer common.CleanupBodyStorage(c)
			c.Set("id", 3)
			c.Set("token_id", 5)
			arkPrefixCacheResponses(c, pilot)
			c.Next()
		}, func(c *gin.Context) {
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			payload, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, body, string(payload))
			c.Status(http.StatusAccepted)
		})
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusAccepted, recorder.Code)
	}
}

func TestArkPrefixCacheRejectsForeignManagedReferencesBeforeRouting(t *testing.T) {
	engine := gin.New()
	pilot := &service.ArkPrefixCachePilot{UserID: 3, TokenID: 5, Model: "seed-lite", Mode: "observe"}
	engine.POST("/v1/responses", func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Set("id", 4)
		c.Set("token_id", 6)
		arkPrefixCacheResponses(c, pilot)
		c.Next()
	}, func(c *gin.Context) { c.Status(http.StatusAccepted) })
	request := httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"seed-lite","previous_response_id":"resp_ark_prefix_foreign"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "cache_not_found")
}

func TestArkPrefixCacheNeverRelaysManagedIDsWithoutPilot(t *testing.T) {
	for _, tt := range []struct {
		id   string
		want int
	}{
		{model.ArkSessionCacheIDPrefix + "local", http.StatusNotFound},
		{model.ArkPrefixCacheIDPrefix + "local", http.StatusNotFound},
		{"resp_from_another_provider", http.StatusAccepted},
	} {
		engine := gin.New()
		engine.POST("/v1/responses", func(c *gin.Context) {
			defer common.CleanupBodyStorage(c)
			arkPrefixCacheResponses(c, nil)
			c.Next()
		}, func(c *gin.Context) { c.Status(http.StatusAccepted) })
		request := httptest.NewRequest(http.MethodPost, "/v1/responses",
			strings.NewReader(`{"model":"seed-lite","previous_response_id":"`+tt.id+`"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, tt.want, recorder.Code)
	}
}

func TestArkPrefixCacheAcceptsTextMessagesWithoutDiscardingUnknownFields(t *testing.T) {
	const valid = `[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]`
	input, ok := arkCacheTextInput(common.RawMessage(valid), 5)
	require.True(t, ok)
	assert.JSONEq(t, valid, string(input))
	for _, invalid := range []string{
		`[{"role":"user","name":"someone","content":"hello"}]`,
		`[{"role":"user","content":[{"type":"input_text","text":"hello","image_url":"other"}]}]`,
		`[{"role":"user","content":[{"type":"input_text","text":"too long"}]}]`,
	} {
		_, ok := arkCacheTextInput(common.RawMessage(invalid), 5)
		assert.False(t, ok, invalid)
	}
}

func TestArkPrefixCacheReplaysTheRecordedProviderRejectionStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	arkCacheReplay(c, "same", model.ArkPrefixCache{RequestDigest: "same", State: model.ArkCacheRejected, RejectedStatus: http.StatusForbidden})
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "cache_provider_rejected")
}

func TestArkPrefixCacheOrphanCleanupCannotCreateASuccessfulReplay(t *testing.T) {
	for _, state := range []string{model.ArkCacheDeleting, model.ArkCacheDeleted} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		arkCacheReplay(c, "same", model.ArkPrefixCache{RequestDigest: "same", State: state, ProviderID: "resp_orphan"})
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "cache_create_outcome_unknown")
	}
}
