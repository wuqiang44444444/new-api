package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerBillingNativeExplanationMatchesActualMetering(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	for _, tc := range []struct {
		name, model  string
		image, audio int
		qpu          float64
	}{
		{"image", "m", 200, 0, 500000},
		{"audio", "gemini-2.5-flash", 0, 200, 500000},
		{"non-default conversion", "m", 0, 0, 1000000},
		{"mixed dimensions", "gemini-2.5-flash", 200, 100, 1000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaPerUnit = tc.qpu
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{OriginModelName: tc.model, StartTime: time.Now(), PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 5, ImageRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 0.5}}, ContractBillingFact: contractBillingFact()}
			usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100}
			usage.PromptTokensDetails.ImageTokens = tc.image
			usage.PromptTokensDetails.AudioTokens = tc.audio
			summary := calculateTextQuotaSummary(ctx, info, usage)
			other := map[string]any{"model_ratio": float64(1), "completion_ratio": float64(5), "group_ratio": 0.5, "contract_discount": "0.8"}
			if tc.image > 0 {
				other["image"] = true
				other["image_output"] = float64(tc.image)
				other["image_ratio"] = float64(2)
			}
			if tc.audio > 0 {
				other["audio_input_seperate_price"] = true
				other["audio_input_token_count"] = float64(tc.audio)
				other["audio_input_price"] = operation_setting.GetGeminiInputAudioPricePerMillionTokens(tc.model)
			}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			log := &model.Log{Type: model.LogTypeConsume, PromptTokens: 1000, CompletionTokens: 100, Quota: summary.Quota, Other: string(raw)}
			lines := customerBillingLines(log, other, model.CustomerBillingLogRow(log), common.QuotaPerUnit)
			require.NotEmpty(t, lines)
			var subtotal float64
			for _, line := range lines {
				subtotal += line.Subtotal
			}
			assert.InDelta(t, float64(summary.Quota)/tc.qpu, subtotal*0.5*0.8, 1/tc.qpu)
			assert.Equal(t, float64(1000-tc.image-tc.audio), lines[0].Quantity)
			assert.Equal(t, 1000000/tc.qpu, lines[0].UnitPrice)
			// An unrecorded additional multiplier must not produce a false equation.
			log.Quota = summary.Quota * 3
			assert.Empty(t, customerBillingLines(log, other, model.CustomerBillingLogRow(log), common.QuotaPerUnit))
		})
	}
}

func TestCustomerExportCleanupRotatesUnavailableStorageJobs(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	for i := 0; i < 6; i++ {
		identity := "old-storage"
		if i == 5 {
			identity = store.ExportIdentity()
		}
		artifact, err := common.Marshal(model.CustomerExportArtifact{StoreIdentity: identity, Files: []model.CustomerExportArtifactFile{{ObjectKey: fmt.Sprintf("file-%d", i)}}})
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.CustomerExportJob{JobID: fmt.Sprintf("cleanup-%d", i), Status: model.CustomerExportJobStatusFailed, Artifact: string(artifact), FinishedAt: common.GetTimestamp()}).Error)
	}
	customerExportCleanupPass(context.Background())
	assert.Empty(t, store.deleted)
	// A fresh scheduler invocation still rotates based on the persistent attempt.
	customerExportCleanupPass(context.Background())
	assert.Contains(t, store.deleted, "file-5")
	var jobs []model.CustomerExportJob
	require.NoError(t, model.DB.Order("id asc").Find(&jobs).Error)
	require.Len(t, jobs, 6)
	for _, job := range jobs[:5] {
		assert.NotEmpty(t, job.Artifact)
	}
	assert.Empty(t, jobs[5].Artifact)
}

func TestCustomerExportScopeFreezePreservesZeroAndSummaryIgnoresFilters(t *testing.T) {
	zero := 0
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	request := CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, TokenId: &zero, ChannelId: &zero, TokenName: "key", Group: "vip", RequestId: "req", UpstreamRequestId: "up", Username: "customer", BillingMode: "token", ModelName: "m"}
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementDetails, request)
	require.NoError(t, err)
	raw, err := common.Marshal(filters)
	require.NoError(t, err)
	job := model.CustomerExportJob{Filters: string(raw)}
	frozen, err := job.DecodeFilters()
	require.NoError(t, err)
	require.NotNil(t, frozen.TokenId)
	assert.Zero(t, *frozen.TokenId)
	assert.Equal(t, filters, frozen)
	summary, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementSummary, request)
	require.NoError(t, err)
	assert.Nil(t, summary.TokenId)
	assert.Nil(t, summary.ChannelId)
	assert.Empty(t, summary.TokenName)
	assert.Empty(t, summary.ModelName)
	assert.Empty(t, summary.BillingMode)
	assert.Empty(t, summary.RequestId)
	assert.Empty(t, summary.UpstreamRequestId)
	assert.Empty(t, summary.Group)
	assert.Empty(t, summary.Username)
}

func TestCustomerBillingExplanationRejectsPartialFrozenUsage(t *testing.T) {
	for _, tc := range []struct {
		name, expression, other string
		prompt                  int
	}{
		{"missing task dimension", `tier("base", u("seconds") * 0.4 + u("requests") * 2)`, `"usage_units":{"seconds":"second","requests":"request"},"usage_facts":{"seconds":5}`, 0},
		{"incomplete cache TTL", `tier("base", p * 2 + cc * 3 + cc1h * 4)`, `"usage_semantic":"anthropic","cache_creation_tokens":100,"cache_creation_tokens_5m":30,"group_ratio":1,"contract_applicable":false`, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"billing_mode":"tiered_expr","matched_tier":"base","expr_b64":"%s",%s}`, base64.StdEncoding.EncodeToString([]byte(tc.expression)), tc.other)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(raw, &other))
			log := &model.Log{Type: model.LogTypeConsume, PromptTokens: tc.prompt, Other: raw}
			assert.Empty(t, customerBillingLines(log, other, model.CustomerBillingLogRow(log), common.QuotaPerUnit))
		})
	}
}
