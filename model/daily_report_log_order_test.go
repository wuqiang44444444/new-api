package model

import (
	"slices"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportLogOrderPreservesPublishedFacts(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&Token{}, &BatchJobLine{}))
	ctx := t.Context()
	end := start + billingReportDaySeconds
	channel := Channel{Id: 21, Name: "report channel", BaseURL: urlPtr("https://provider.example/v1")}
	require.NoError(t, db.Create(&channel).Error)
	task := Task{TaskID: "report-video", Platform: "seedance", UserId: 7, ChannelId: channel.Id, Status: TaskStatusSuccess,
		FinishTime: start + 100, Quota: 180, Properties: Properties{OriginModelName: "video"}, BillingState: TaskBillingStateSettled,
		PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
			State: TaskBillingStateSettled, Operation: "settle", ActualUsageReported: true,
			TieredSnapshot: &billingexpr.BillingSnapshot{UsageUnits: map[string]string{"duration": "second"}, UsageFacts: map[string]any{"duration": 6}},
		}},
	}
	require.NoError(t, db.Create(&task).Error)

	contract := `"group_ratio":0.8,"contract_applicable":true,"contract_discount":0.9,"contract_id":3,"contract_version":1`
	perSecond := `{"task_id":"report-video","contract_applicable":false,"group_ratio":1,"usage_units":{"duration":"second"},"statement_snapshot":{"billing_mode":"per_second","customer_model":"video"},"admin_info":{"statement_original_quota":100}}`
	logs := []Log{
		{UserId: 7, ChannelId: channel.Id, TokenId: 11, TokenName: "old key", ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 30, Quota: 100, PromptTokens: 10, RequestId: "charge-old",
			Other: `{` + contract + `,"contract_name":"original contract","admin_info":{"statement_original_quota":140}}`},
		{UserId: 7, ChannelId: channel.Id, TokenId: 11, ModelName: "text", Type: LogTypeRefund, CreatedAt: start + 20, Quota: 30, PromptTokens: 1, RequestId: "refund",
			Other: `{` + contract + `,"contract_name":"original contract","admin_info":{"statement_original_quota":40}}`},
		{UserId: 7, ChannelId: channel.Id, TokenId: 41, TokenName: "initial video key", ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 50, Quota: 100, RequestId: "video-old", Other: perSecond},
		{UserId: 7, ChannelId: channel.Id, TokenId: 11, TokenName: "renamed key", ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 80, PromptTokens: 8, RequestId: "charge-new",
			Other: `{` + contract + `,"contract_name":"renamed contract","admin_info":{"statement_original_quota":110}}`},
		{UserId: 7, ChannelId: channel.Id, TokenId: 52, TokenName: "another video key", ModelName: "video", Type: LogTypeConsume, CreatedAt: start, Quota: 80, RequestId: "video-new", Other: perSecond},
		{UserId: 7, ChannelId: channel.Id, TokenId: 11, TokenName: "outside the day", ModelName: "text", Type: LogTypeConsume, CreatedAt: end, Quota: 900},
		{UserId: 7, ChannelId: channel.Id, TokenId: 11, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 10, Quota: 25, PromptTokens: 2, RequestId: "different contract",
			Other: `{"group_ratio":0.7,"contract_applicable":true,"contract_discount":0.8,"contract_id":4,"contract_version":1,"contract_name":"another contract","admin_info":{"statement_original_quota":35}}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	// Fill exactly one timestamp page. Its last ID is the global upper bound,
	// while lower IDs at later timestamps still belong to the same report.
	firstPage := make([]Log, 498)
	for i := range firstPage {
		firstPage[i] = Log{UserId: 8, ChannelId: channel.Id, ModelName: "text", Type: LogTypeConsume, CreatedAt: start}
	}
	require.NoError(t, db.CreateInBatches(&firstPage, 100).Error)
	bounded, err := WithDailyReportLogBoundary(ctx)
	require.NoError(t, err)
	upper, err := dailyReportLogUpperBound(bounded)
	require.NoError(t, err)
	require.EqualValues(t, firstPage[len(firstPage)-1].Id, upper)

	wantBilling, err := GetBillingCustomerStatement(ctx, 7, start, end-1, "api_key", 0, "", "")
	require.NoError(t, err)
	wantProvider, err := getProviderBillingURLSummary(ctx, start, end-1, start, "", 0)
	require.NoError(t, err)
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	wantUsage, err := runUsageAggregation(ctx, period, usageViewOptions{WantCustomer: true, WantUpstream: true, LogUpper: &upper})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{UserId: 7, ChannelId: channel.Id, TokenId: 11, TokenName: "after boundary",
		ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 900}).Error)

	policy := BillingStatementReadPolicy{ReadOnlySource: true, UpperLogID: &upper}
	filter := UpstreamBillingDetailFilter{Start: start, End: end - 1}
	var legacyIDs, dailyIDs []int
	require.NoError(t, scanUpstreamBillingFacts(ctx, filter, policy, func(log Log, _ billingReconciliationLog, _ parsedBillingReconciliationLog) error {
		legacyIDs = append(legacyIDs, log.Id)
		return nil
	}))
	require.NoError(t, scanUpstreamBillingFacts(withDailyReportLogOrder(ctx), filter, policy, func(log Log, _ billingReconciliationLog, _ parsedBillingReconciliationLog) error {
		dailyIDs = append(dailyIDs, log.Id)
		return nil
	}))
	require.Len(t, dailyIDs, 504)
	assert.True(t, slices.IsSorted(legacyIDs), "read-only inventory keeps its ID order")
	assert.ElementsMatch(t, legacyIDs, dailyIDs)
	if db.Dialector.Name() == "postgres" {
		assert.EqualValues(t, upper, dailyIDs[499])
		assert.Equal(t, []int{logs[6].Id, logs[1].Id, logs[0].Id, logs[2].Id}, dailyIDs[500:])
	} else {
		assert.Equal(t, legacyIDs, dailyIDs)
	}

	staged := StageDailyReport(bounded)
	require.NoError(t, BuildBillingReportDay(staged, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildUsageReportDay(staged, date, "billing-test", "runner", nil))
	require.NoError(t, PublishDailyReportStages(staged, []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}, "billing-test", "runner"))

	pinned, report, err := ResolveBillingReport(ctx, start, end-1, "")
	require.NoError(t, err)
	actualBilling, err := GetBillingCustomerStatement(pinned, 7, start, end-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.Equal(t, wantBilling, actualBilling)
	assert.EqualValues(t, 355, actualBilling.Summary.NetQuota)
	var charges []billingReportLine
	require.NoError(t, db.Where("generation IN ? AND user_id = ?", report.ids(), 7).Order("source_id ASC").Find(&charges).Error)
	require.Len(t, charges, 6)
	assert.EqualValues(t, logs[1].Id, charges[1].SourceID)
	assert.EqualValues(t, -30, charges[1].CustomerCharge)
	var customer billingReportCustomerDay
	require.NoError(t, db.Where("generation IN ? AND user_id = ?", report.ids(), 7).Take(&customer).Error)
	payload, err := dailyReportPayload(ctx, db, customer.Generation, reportBillingCustomerPayload, 7, customer.Leaves)
	require.NoError(t, err)
	var details billingReportCustomerDetails
	require.NoError(t, common.UnmarshalJsonStr(payload, &details))
	assert.Equal(t, "renamed key", details.Key.Names[11], "an empty name on a newer event cannot replace the last recorded name")
	var textPrices []billingReportPrice
	for _, leaf := range details.Key.Leaves {
		if leaf.GroupID == 11 && leaf.Price.Model.ModelName == "text" {
			textPrices = append(textPrices, leaf.Price)
		}
	}
	require.Len(t, textPrices, 1)
	require.NotNil(t, textPrices[0].GroupRatio)
	require.NotNil(t, textPrices[0].ContractRatio)
	assert.Equal(t, 0.8, *textPrices[0].GroupRatio)
	assert.Equal(t, 0.9, *textPrices[0].ContractRatio)
	assert.True(t, textPrices[0].Model.MultipleDiscounts)
	assert.True(t, textPrices[0].MultipleContract)

	actualProvider, err := getProviderBillingURLSummary(pinned, start, end-1, start, "", 0)
	require.NoError(t, err)
	require.Len(t, actualProvider.Groups, 1)
	require.Len(t, wantProvider.Groups, 1)
	assert.Equal(t, wantProvider.Groups[0].Usage, actualProvider.Groups[0].Usage)
	projections, err := report.upstreamRows(ctx, []int{channel.Id})
	require.NoError(t, err)
	require.Len(t, projections, 1)
	var evidence []billingRecoveredSeconds
	for _, projectedTask := range projections[0].Tasks {
		if projectedTask.TaskID == task.TaskID && projectedTask.Recovered != nil {
			evidence = append(evidence, *projectedTask.Recovered)
		}
	}
	require.Len(t, evidence, 1)
	assert.EqualValues(t, logs[2].Id, evidence[0].SourceID)
	assert.Equal(t, "6", evidence[0].Value.String())

	actualUsage, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	require.Len(t, meta.References, 1)
	for key := range wantUsage.customer {
		if key.user != 7 {
			delete(wantUsage.customer, key)
		}
	}
	expectedUsage := UsageCustomerView{Keys: []UsageCustomerKeyGroup{}}
	finalizeUsageCustomerView(&expectedUsage, wantUsage)
	assert.Equal(t, expectedUsage, actualUsage)
}

func TestDailyReportTaskLogsKeepAPIKeyAndMeterPrecedence(t *testing.T) {
	agg := usageAggregation{}
	pieces := &usageTaskPieces{}
	laterImages, earlierImages, deliveredImages := int64(3), int64(1), int64(2)
	// The higher ID arrives first in time order. The first known mode and
	// API key still come from the lower ID, and the last count from the higher.
	agg.appendTaskLogPiece(pieces, usageLogRow{
		id: 20, fact: billingReconciliationLog{TokenId: 9, TokenName: "adjustment key"},
		parsed: parsedBillingReconciliationLog{billingMode: BillingReconciliationModePerImage, billableImageApplicable: true, billableImageCount: &laterImages},
	}, nil)
	agg.appendTaskLogPiece(pieces, usageLogRow{
		id: 10, fact: billingReconciliationLog{TokenId: 7, TokenName: "original key"},
		parsed: parsedBillingReconciliationLog{billingMode: BillingReconciliationModePerCall, billableImageApplicable: true, billableImageCount: &earlierImages},
	}, nil)
	assert.Equal(t, BillingReconciliationModePerCall, pieces.billingMode)
	assert.Equal(t, 7, pieces.tokenId)
	assert.Equal(t, "original key", pieces.tokenName)
	require.NotNil(t, pieces.billableImageCount)
	assert.Equal(t, laterImages, *pieces.billableImageCount)

	// Direct delivery evidence is merged after the range scan.
	agg.appendTaskLogPiece(pieces, usageLogRow{
		fact:   billingReconciliationLog{TokenId: 12, TokenName: "delivery key"},
		parsed: parsedBillingReconciliationLog{billingMode: BillingReconciliationModePerImage, billableImageApplicable: true, billableImageCount: &deliveredImages},
	}, nil)
	assert.Equal(t, BillingReconciliationModePerCall, pieces.billingMode)
	assert.Equal(t, 7, pieces.tokenId)
	require.NotNil(t, pieces.billableImageCount)
	assert.Equal(t, deliveredImages, *pieces.billableImageCount)
}
