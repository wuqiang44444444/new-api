package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func submitNativeImageCreate(t *testing.T, engine *gin.Engine, path, body string, prefer bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if prefer {
		request.Header.Set("Prefer", "respond-async")
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

// nativeDiagFixture wires one OpenAI-type native image channel against a
// scripted provider, plus the standard auth + idempotency + relay chain.
type nativeDiagFixture struct {
	db     *gorm.DB
	user   model.User
	token  model.Token
	engine *gin.Engine
	calls  *atomic.Int32
}

func newNativeDiagFixture(t *testing.T, status int, body string, headers map[string]string) *nativeDiagFixture {
	t.Helper()
	db := nativeImageTestDB(t)
	user := model.User{Id: 8961, Username: "native-diag", Quota: 100000, Group: "default", Status: common.UserStatusEnabled}
	token := model.Token{Id: 8961, UserId: user.Id, Key: strings.Repeat("d", 32), RemainQuota: 100000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		for key, value := range headers {
			w.Header().Set(key, value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(provider.Close)
	channel := model.Channel{Id: 8961, Type: constant.ChannelTypeOpenAI, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	engine := gin.New()
	engine.Use(middleware.ImageErrorEvidence())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-image-2"))
		c.Next()
	}, middleware.ImageCreateIdempotency(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
	return &nativeDiagFixture{db: db, user: user, token: token, engine: engine, calls: &calls}
}

type imageDiagBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *imageDiagBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *imageDiagBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
func waitImageDiagDelivery(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		h := service.ImageTaskDiagHealth()
		return h.Accepted == h.Written+h.Failed
	}, 3*time.Second, 10*time.Millisecond)
}
func captureNativeImageDiag(t *testing.T) *imageDiagBuffer {
	t.Helper()
	waitImageDiagDelivery(t)
	buffer := &imageDiagBuffer{}
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = buffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		waitImageDiagDelivery(t)
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})
	return buffer
}

func waitNativeImageDiagLine(t *testing.T, buffer *imageDiagBuffer, taskID string) string {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(buffer.String(), "event=image_task_execution_diag task_id="+taskID)
	}, 3*time.Second, 20*time.Millisecond, buffer.String())
	for _, line := range strings.Split(buffer.String(), "\n") {
		if strings.Contains(line, "event=image_task_execution_diag task_id="+taskID) {
			return line
		}
	}
	return ""
}

// Phase-3 acceptance: a definite provider rejection persists bounded facts,
// aligns the existing violation-fee policy with the sync error exit, refunds
// through the idempotent settlement, and emits exactly one sanitized
// observation event. Unknown outcomes keep funds held with the same evidence.
func TestNativeImageRejectionEvidenceFeeAndDiag(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body          string
		headers       map[string]string
		disableFee    bool
		zeroRatio     bool
		changePolicy  bool
		wantStatus    model.TaskStatus
		wantUserQuota int
		wantEvidence  model.TaskImageFailureEvidence
		wantStage     string
		wantTrusted   string
	}{
		{
			name:          "violation_marker_charges_like_sync",
			status:        400,
			body:          `{"error":{"message":"request rejected: Failed check: SAFETY_CHECK_TYPE"}}`,
			headers:       map[string]string{"X-Request-Id": "req-vio-1"},
			wantStatus:    model.TaskStatusFailure,
			wantUserQuota: 75000, // hold 20000; settlement target = fee 25000
			wantEvidence:  model.TaskImageFailureEvidence{UpstreamStatus: 400, ProviderRequestID: "req-vio-1", ViolationMarker: true},
			wantStage:     "stage=provider_rejected",
			wantTrusted:   "trusted=true",
		},
		{
			name: "zero_ratio_keeps_fee_zero", status: 400,
			body: `{"error":{"message":"Content violates usage guidelines"}}`, zeroRatio: true,
			wantStatus: model.TaskStatusFailure, wantUserQuota: 100000,
			wantEvidence: model.TaskImageFailureEvidence{UpstreamStatus: 400, ViolationMarker: true},
			wantStage:    "stage=provider_rejected", wantTrusted: "trusted=true",
		},
		{
			name: "accepted_policy_survives_changes", status: 400,
			body: `{"error":{"message":"Content violates usage guidelines"}}`, changePolicy: true,
			wantStatus: model.TaskStatusFailure, wantUserQuota: 75000,
			wantEvidence: model.TaskImageFailureEvidence{UpstreamStatus: 400, ViolationMarker: true},
			wantStage:    "stage=provider_rejected", wantTrusted: "trusted=true",
		},
		{
			name:          "ordinary_rejection_refunds_without_fee",
			status:        400,
			body:          `{"error":{"message":"invalid value: quality"}}`,
			headers:       map[string]string{"X-Request-Id": "req-plain-1"},
			wantStatus:    model.TaskStatusFailure,
			wantUserQuota: 100000,
			wantEvidence:  model.TaskImageFailureEvidence{UpstreamStatus: 400, ProviderRequestID: "req-plain-1"},
			wantStage:     "stage=provider_rejected",
			wantTrusted:   "trusted=true",
		},
		{
			name:          "policy_disabled_keeps_zero_target",
			status:        400,
			body:          `{"error":{"message":"Content violates usage guidelines"}}`,
			disableFee:    true,
			changePolicy:  true,
			wantStatus:    model.TaskStatusFailure,
			wantUserQuota: 100000,
			wantEvidence:  model.TaskImageFailureEvidence{UpstreamStatus: 400, ViolationMarker: true},
			wantStage:     "stage=provider_rejected",
			wantTrusted:   "trusted=true",
		},
		{
			name:          "server_error_keeps_hold_with_evidence",
			status:        502,
			body:          `{}`,
			headers:       map[string]string{"X-Ms-Request-Id": "azure-502"},
			wantStatus:    model.TaskStatusReconciliationRequired,
			wantUserQuota: 80000,
			wantEvidence:  model.TaskImageFailureEvidence{UpstreamStatus: 502, ProviderRequestID: "azure-502"},
			wantStage:     "stage=native_image_outcome_unknown",
			wantTrusted:   "trusted=false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldGrok := *model_setting.GetGrokSettings()
			model_setting.GetGrokSettings().ViolationDeductionEnabled = true
			model_setting.GetGrokSettings().ViolationDeductionAmount = 0.05
			t.Cleanup(func() { *model_setting.GetGrokSettings() = oldGrok })
			if tc.disableFee {
				model_setting.GetGrokSettings().ViolationDeductionEnabled = false
			}
			fixture := newNativeDiagFixture(t, tc.status, tc.body, tc.headers)
			diag := captureNativeImageDiag(t)
			if tc.zeroRatio {
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0}`))
			}

			recorder := submitNativeImageCreate(t, fixture.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`, true)
			require.Equal(t, 202, recorder.Code, recorder.Body.String())
			var accepted struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))
			require.NotEmpty(t, accepted.ID)
			if tc.changePolicy {
				model_setting.GetGrokSettings().ViolationDeductionEnabled = tc.disableFee
				model_setting.GetGrokSettings().ViolationDeductionAmount = 999
			}

			service.RunImageTaskWorkerOnce(context.Background())
			service.RunImageTaskWorkerOnce(context.Background())

			var task model.Task
			require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
			assert.EqualValues(t, tc.wantStatus, task.Status)
			data := task.PrivateData.ImageTask
			require.NotNil(t, data)
			assert.Equal(t, tc.wantEvidence.UpstreamStatus, data.FailureStatus)
			assert.Equal(t, tc.wantEvidence.ProviderRequestID, data.ProviderRequestID)
			assert.Equal(t, tc.wantEvidence.ViolationMarker, data.ViolationMarker)
			if task.Status == model.TaskStatusFailure {
				log, err := service.BuildTaskBillingDeliveryLog(&task, model.TaskBillingDelivery{Event: "complete", AfterQuota: task.Quota})
				require.NoError(t, err)
				var other map[string]any
				require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
				if task.Quota > 0 {
					assert.Equal(t, true, other["violation_fee"])
				} else {
					assert.NotContains(t, other, "violation_fee")
				}
			}

			require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
			require.NoError(t, fixture.db.First(&fixture.token, fixture.token.Id).Error)
			assert.Equal(t, tc.wantUserQuota, fixture.user.Quota)
			assert.Equal(t, tc.wantUserQuota, fixture.token.RemainQuota)

			// Repeated passes must never charge or refund a second time.
			service.RunImageTaskWorkerOnce(context.Background())
			require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
			assert.Equal(t, tc.wantUserQuota, fixture.user.Quota)

			line := waitNativeImageDiagLine(t, diag, accepted.ID)
			assert.Contains(t, line, tc.wantStage)
			assert.Contains(t, line, tc.wantTrusted)
			assert.NotContains(t, diag.String(), "SAFETY_CHECK_TYPE")
			assert.NotContains(t, diag.String(), "Content violates usage guidelines")
			private, err := common.Marshal(task.PrivateData)
			require.NoError(t, err)
			assert.NotContains(t, string(private), "SAFETY_CHECK_TYPE")
			assert.NotContains(t, string(private), "Content violates usage guidelines")
		})
	}
}

// Phase-4 acceptance (§7.2): a client disconnect after the 202 admission must
// never cancel the accepted task; the background worker completes it with the
// frozen facts and settles exactly the frozen target.
func TestNativeImageTaskSurvivesClientDisconnect(t *testing.T) {
	successBody := `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":12,"output_tokens":20,"total_tokens":32}}`
	fixture := newNativeDiagFixture(t, 200, successBody, nil)
	server := httptest.NewServer(fixture.engine)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2","prompt":"draw"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "respond-async")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.Equal(t, 202, response.StatusCode)
	var accepted struct {
		ID string `json:"id"`
	}
	responseBytes, readErr := io.ReadAll(response.Body)
	require.NoError(t, readErr)
	response.Body.Close()
	require.NoError(t, common.Unmarshal(responseBytes, &accepted))
	cancel() // the client is gone; the accepted task must continue

	service.RunImageTaskWorkerOnce(context.Background())
	service.RunImageTaskWorkerOnce(context.Background())

	var task model.Task
	require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Equal(t, 20000, task.Quota)
	require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
	assert.Equal(t, 80000, fixture.user.Quota)
}

// Phase-4 acceptance (§7.2 配置变化): settlement reads the frozen price
// snapshot; a later price change must never reinterpret an in-flight task.
func TestNativeImageTaskSettlesFrozenPriceAfterPriceChange(t *testing.T) {
	successBody := `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":12,"output_tokens":20,"total_tokens":32}}`
	fixture := newNativeDiagFixture(t, 200, successBody, nil)
	recorder := submitNativeImageCreate(t, fixture.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`, true)
	require.Equal(t, 202, recorder.Code, recorder.Body.String())
	var accepted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-2":0.4}`))
	service.RunImageTaskWorkerOnce(context.Background())

	var task model.Task
	require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Equal(t, 20000, task.Quota, "settlement must use the frozen 0.04 price, not the changed 0.4")
	require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
	assert.Equal(t, 80000, fixture.user.Quota)
}

// Phase-2/4 acceptance (§7.2 输出明细/价格一致): the measured Azure usage
// shape settles through the frozen expression with img_o priced separately
// and c excluding the subclass exactly once — the same rule the sync path
// applies to the identical usage payload.
func TestNativeImageTaskSettlesOutputDetailsUnderExpression(t *testing.T) {
	usageBody := `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":19,"output_tokens":196,"total_tokens":215,"output_tokens_details":{"image_tokens":196}}}`
	fixture := newNativeDiagFixture(t, 200, usageBody, nil)
	// 在 nativeImageTestDB 的空配置之后启用表达式计费，模拟管理员先建渠道
	// 后配价的顺序。
	withTieredBillingConfig(t,
		map[string]string{"gpt-image-2": "tiered_expr"},
		map[string]string{"gpt-image-2": `tier("base", p * 2.5 + c * 15 + img_o * 30)`})

	recorder := submitNativeImageCreate(t, fixture.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`, true)
	require.Equal(t, 202, recorder.Code, recorder.Body.String())
	var accepted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))

	service.RunImageTaskWorkerOnce(context.Background())
	service.RunImageTaskWorkerOnce(context.Background())

	var task model.Task
	require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	usage := task.PrivateData.ImageTask.Usage
	require.NotNil(t, usage)
	assert.Equal(t, 196, usage.CompletionTokens)
	assert.Equal(t, 196, usage.CompletionTokenDetails.ImageTokens)
	// (19*2.5 + (196-196)*15 + 196*30) / 1_000_000 * 500000 = 2963.75 → 2964
	assert.Equal(t, 2964, task.Quota)
	require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
	assert.Equal(t, 97036, fixture.user.Quota)
}
