package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementImageCompletionPreservesCachedUsageWithoutDoubleSettlement(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 1830, 100000)
	seedToken(t, 1831, 1830, "image-statement-token", 100000)
	seedChannel(t, 42)
	task := newWorkerImageTask(1830, 300)
	task.Properties.OriginModelName = "public-image"
	task.PrivateData.TokenId = 1831
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "public-image", GroupRatio: 1, ModelRatio: 1,
		ContractFact: &hosttypes.ContractBillingFact{UserId: 1830, PublicModel: "public-image", ContractVersion: 1, RatioUnits: 80000000}}
	task.PrivateData.ImageTask.Price = &hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1,
		CacheCreationRatio: 1.25, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	won, err := model.FinishImageTaskSuccess(task, []model.TaskImageArtifact{{ObjectKey: "test-statement-image"}},
		&dto.Usage{PromptTokens: 1000, CompletionTokens: 20, TotalTokens: 1020,
			PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 800, CacheWriteTokens: 100}})
	require.NoError(t, err)
	require.True(t, won)
	settleImageTaskBilling(context.Background(), task)
	settleImageTaskBilling(context.Background(), reloadTask(t, task.ID))
	DeliverTaskBillingLogs(context.Background(), task.ID, 10)
	statement, err := model.GetBillingCustomerStatement(context.Background(), 1830, 1, common.GetTimestamp()+10, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, statement.Summary.Requests)
	assert.EqualValues(t, 1000, statement.Summary.InputTokens)
	assert.EqualValues(t, 800, statement.Summary.CacheReadTokens)
	assert.EqualValues(t, 100, statement.Summary.CacheWriteTokens)
	assert.EqualValues(t, 260, statement.Summary.NetQuota)
	assert.Equal(t, 99740, getUserQuota(t, 1830))
	assert.Equal(t, 99740, getTokenRemainQuota(t, 1831))
}

func TestStatementTextSettlementPreservesCachedUsageAndContractCharge(t *testing.T) {
	for _, semantic := range []string{"openai", "anthropic", "legacy_claude_conversion"} {
		t.Run(semantic, func(t *testing.T) {
			truncate(t)
			seedUser(t, 1820, 100000)
			seedToken(t, 1821, 1820, "statement-test-token", 100000)
			seedChannel(t, 1822)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set("token_name", "statement-key")
			info := &relaycommon.RelayInfo{
				UserId: 1820, UserQuota: 100000, TokenId: 1821, TokenKey: "statement-test-token",
				UsingGroup: "default", UserGroup: "default", TokenGroup: "default",
				OriginModelName: "public-text", BillingModelName: "price-text",
				StartTime: time.Now(), FirstResponseTime: time.Now(),
				ChannelMeta:         &relaycommon.ChannelMeta{ChannelId: 1822, ChannelType: constant.ChannelTypeOpenAI},
				ContractBillingFact: &hosttypes.ContractBillingFact{UserId: 1820, PublicModel: "public-text", ContractVersion: 1, RatioUnits: 80000000},
				PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1, CacheCreationRatio: 1.25,
					CacheCreation5mRatio: 1.25, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 20, TotalTokens: 1020,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 800, CachedCreationTokens: 100}}
			if semantic != "openai" {
				usage.PromptTokens = 100
				if semantic == "anthropic" {
					usage.UsageSemantic = "anthropic"
				} else {
					usage.ClaudeCacheCreation5mTokens = 100
				}
			}
			require.Nil(t, PreConsumeBilling(c, 300, info))
			PostTextConsumeQuota(c, info, usage, nil)
			statement, err := model.GetBillingCustomerStatement(context.Background(), 1820, 1, common.GetTimestamp()+10, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, 1000, statement.Summary.InputTokens)
			assert.EqualValues(t, 800, statement.Summary.CacheReadTokens)
			assert.EqualValues(t, 100, statement.Summary.CacheWriteTokens)
			assert.EqualValues(t, 260, statement.Summary.NetQuota)
			assert.Equal(t, 99740, getUserQuota(t, 1820))
			assert.Equal(t, 99740, getTokenRemainQuota(t, 1821))
			assert.Zero(t, statement.DataQuality.InputTokensUnavailableRequests)
		})
	}
}

func TestStatementBatchKeepsInt64UsageAcrossSettlementAndDetail(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	created, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	job := created.Job
	// Exercise the persisted line-fact boundary, independently of provider I/O.
	require.NoError(t, model.DB.Model(job).Update("upstream_status", "completed").Error)
	lines := []model.BatchJobLine{
		{JobId: job.Id, CustomId: "a", Status: "completed", InputTokens: 1300000000, OutputTokens: 1500000000, CachedTokens: 1100000000, TotalTokens: 2800000000, FinalQuota: 10},
		{JobId: job.Id, CustomId: "b", Status: "completed", InputTokens: 1300000000, OutputTokens: 1500000000, CachedTokens: 1100000000, TotalTokens: 2800000000, FinalQuota: 10},
	}
	require.NoError(t, model.CommitBatchResultLines(job, lines))
	target, err := settleBatchJobTarget(job)
	require.NoError(t, err)
	require.Equal(t, 20, target)
	require.NoError(t, applyBatchSettlement(context.Background(), job, target))
	require.NoError(t, applyBatchSettlement(context.Background(), job, target))
	statement, err := model.GetBillingCustomerStatement(context.Background(), 1701, 1, common.GetTimestamp()+10, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2600000000, statement.Summary.InputTokens)
	assert.EqualValues(t, 3000000000, statement.Summary.OutputTokens)
	assert.EqualValues(t, 2200000000, statement.Summary.CacheReadTokens)
	assert.EqualValues(t, 2, statement.Summary.Requests)
	assert.EqualValues(t, 20, statement.Summary.NetQuota)
	assert.Equal(t, 99980, getUserQuota(t, 1701))
	detail, err := model.GetBillingStatementLogs(context.Background(), model.BillingStatementLogFilter{UserId: 1701, Start: 1, End: common.GetTimestamp() + 10}, 1, 10, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, detail.Items, 1)
	assert.EqualValues(t, 2600000000, detail.Items[0].PromptTokens)
	assert.EqualValues(t, 3000000000, detail.Items[0].CompletionTokens)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderBillingDiscount{}, &model.ProviderBillingAudit{}))
	provider, err := model.GetProviderBillingSummary(1, common.GetTimestamp()+10, 1, 1701, "", "", 1)
	require.NoError(t, err)
	require.Len(t, provider.Channels, 1)
	assert.EqualValues(t, 2600000000, provider.Channels[0].Usage.InputTokens)
	assert.EqualValues(t, 3000000000, provider.Channels[0].Usage.OutputTokens)
	assert.EqualValues(t, 2200000000, provider.Channels[0].Usage.CacheReadTokens)
}
