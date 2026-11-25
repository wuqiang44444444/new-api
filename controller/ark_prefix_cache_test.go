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
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestPaidArkSessionLeavesUnmanagedResponsesToNativeRelay(t *testing.T) {
	engine := gin.New()
	pilot := &service.ArkPrefixCachePilot{Mode: "charge", Model: "seed-lite", MeterVersion: setting.ArkSessionPlatformStorageMeter}
	engine.POST("/v1/responses", func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Set("id", 3)
		c.Set("token_id", 5)
		arkPrefixCacheResponses(c, pilot)
		c.Next()
	}, func(c *gin.Context) { c.Status(http.StatusAccepted) })
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{"upstream continuation", `{"model":"seed-lite","previous_response_id":"resp_upstream","caching":{"type":"enabled"},"store":true}`, http.StatusAccepted},
		{"ordinary model", `{"model":"other","previous_response_id":"resp_upstream","caching":{"type":"enabled"}}`, http.StatusAccepted},
		{"ordinary seed request", `{"model":"seed-lite","previous_response_id":"resp_upstream"}`, http.StatusAccepted},
		{"fixed prefix", `{"model":"seed-lite","input":"text","caching":{"type":"enabled","prefix":true}}`, http.StatusAccepted},
		{"explicit store false and SSE", `{"model":"seed-lite","input":"text","caching":{"type":"enabled"},"store":false,"stream":true}`, http.StatusAccepted},
		{"stored native request without expiry", `{"model":"seed-lite","input":"text","caching":{"type":"enabled"},"store":true}`, http.StatusAccepted},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(test.body)))
			assert.Equal(t, test.want, recorder.Code)
		})
	}
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

func TestPaidArkSessionKeepsHistoricalPrefixReplayInManagedHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ark-replay.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	original := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = original
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.ArkPrefixCache{}))
	const payload = `{"model":"seed-lite","input":"saved","store":true,"expire_at":120,"caching":{"type":"enabled","prefix":true},"thinking":{"type":"disabled"}}`
	digest := model.ArkPrefixCacheKeyDigest("historical-prefix")
	prior := model.ArkPrefixCache{ID: model.ArkPrefixCacheIDPrefix + "historical", UserID: 3, TokenID: 5,
		Model: "seed-lite", KeyDigest: digest, RequestDigest: service.ArkCacheDigest(payload),
		State: model.ArkCacheActive, CreatedAt: 60, ExpiresAt: 120, ProviderID: "resp_test"}
	require.NoError(t, db.Create(&prior).Error)
	pilot := &service.ArkPrefixCachePilot{Mode: "charge", Model: prior.Model, MeterVersion: setting.ArkSessionPlatformStorageMeter, MaxInputBytes: 8192}
	engine := gin.New()
	engine.POST("/v1/responses", func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Set("id", prior.UserID)
		c.Set("token_id", prior.TokenID)
		c.Set(service.ArkCacheReplayOnlyKey, digest)
		arkPrefixCacheResponses(c, pilot)
	}, func(c *gin.Context) { c.Status(http.StatusAccepted) })
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.Header.Set("Idempotency-Key", "historical-prefix")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	assert.Contains(t, res.Body.String(), prior.ID)
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
