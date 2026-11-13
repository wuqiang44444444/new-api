package service

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenImageBillingReport(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	ctx := context.Background()
	period, err := model.ResolveUsageAnalyticsPeriod("day", "2026-09-16", common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 229, Name: "image channel"}).Error)

	const expression = `p > 5 ? tier("token", p*10+c*20) : tier("image", fixed(0.04))*image_count`
	newLog := func(id, unit, tier string, quota int, fields map[string]any) model.Log {
		fields["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(expression))
		fields["billing_mode"], fields["billing_unit"], fields["matched_tier"] = "tiered_expr", unit, tier
		fields["group_ratio"], fields["contract_applicable"] = 1, false
		fields["request_path"] = "/v1/images/edits"
		other, err := common.Marshal(fields)
		require.NoError(t, err)
		return model.Log{UserId: 42, TokenId: 99, ChannelId: 229, Type: model.LogTypeConsume,
			ModelName: "image-model", CreatedAt: period.StartTimestamp + 1, RequestId: id, Quota: quota,
			PromptTokens: 900, CompletionTokens: 1900, Other: string(other)}
	}
	// Token-bearing telemetry on the image call is unrelated to its charge.
	// A conditional's p input is preserved so the same tier can be replayed.
	image := newLog("charged-image", "request", "image", 40000, map[string]any{
		"image_count": 2, "completed_image_count": 1, "billing_tokens": map[string]int{"p": 0, "c": 0},
	})
	token := newLog("charged-token", "token", "token", 90, map[string]any{
		"image_count": 8, "completed_image_count": 3, "billing_tokens": map[string]int{"p": 8, "c": 1},
	})
	test := newLog("test-image", "request", "image", 20000, map[string]any{
		"image_count": 1, "billing_tokens": map[string]int{"p": 0, "c": 0},
	})
	test.TokenId, test.TokenName, test.Content = 0, "Model test", "Model test"
	missing := newLog("missing", "request", "image", 20000, map[string]any{
		"completed_image_count": 1, "billing_tokens": map[string]int{"p": 0, "c": 0},
	})
	require.NoError(t, model.DB.Create(&[]model.Log{image, token, test, missing}).Error)
	prepareDailyReportExportTest(t)
	require.NoError(t, model.BuildUsageReportDay(ctx, period.Date, "export-fixture", "runner", nil))
	require.NoError(t, model.BuildBillingReportDay(ctx, period.StartTimestamp, "export-fixture", "runner", nil, nil))
	_, meta, err := model.GetPublishedUsageUpstream(ctx, period, model.UsageUpstreamScope{})
	require.NoError(t, err)
	channel := 229
	frozen := model.WithUsageReportReferences(ctx, meta.References)
	upstream, _, err := model.GetPublishedUsageUpstream(frozen, period, model.UsageUpstreamScope{ChannelID: &channel})
	require.NoError(t, err)
	assert.EqualValues(t, 2, upstream.Total.BillableImages)
	assert.EqualValues(t, 1, upstream.Total.TestBillableImages)
	assert.EqualValues(t, 1, upstream.Total.BillableImageMissingRows)
	assert.EqualValues(t, 5, upstream.Total.ImageCount)
	assert.True(t, upstream.Total.BillingTokensApplicable)
	assert.EqualValues(t, 9, upstream.Total.InputTokens+upstream.Total.OutputTokens, "only the token-priced request contributes")
	assert.Equal(t, upstream.Total, upstream.DayTotals[0])
	customer, _, err := model.GetPublishedUsageCustomer(frozen, period, 42, "")
	require.NoError(t, err)
	assert.EqualValues(t, 60090, customer.Total.NetQuota)

	rank, err := model.GetPublishedUsageRanks(frozen, period, model.UsageReportRankQuery{Kind: "channel", Metric: "calls", Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, rank.Items, 1)
	require.NotNil(t, rank.Items[0].NonToken)
	assert.Equal(t, upstream.Total.BillingImageTotals, rank.Items[0].NonToken.BillingImageTotals)
	assert.EqualValues(t, 9, rank.Items[0].Tokens)
	assert.True(t, rank.Items[0].NonToken.BillingTokensApplicable)

	filters := model.CustomerExportFilters{FieldVersion: usageAnalyticsExportFieldVersion, ChannelId: &channel, UsageReportDays: meta.References}
	rows, _, err := usageAnalyticsExportRows(ctx, &model.CustomerExportJob{}, usageAnalyticsExportTypeUpstream, period, filters, customerExportScopeColumns{})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	total := rows[len(rows)-1]
	assert.Equal(t, []string{"2", "1", "1", "", ""}, total[len(total)-5:])
	var imageTotal, tokenTotal bool
	for _, row := range rows {
		switch {
		case row[0] == "period_total" && row[12] == "per_image":
			imageTotal = true
			assert.Empty(t, row[18])
			assert.Empty(t, row[19])
			assert.Equal(t, "2", row[len(row)-5])
		case row[0] == "period_total" && row[12] == "token":
			tokenTotal = true
			assert.Equal(t, "8", row[18])
			assert.Equal(t, "1", row[19])
			assert.Equal(t, "3", row[22])
			assert.Empty(t, row[len(row)-5])
		}
	}
	assert.True(t, imageTotal && tokenTotal)
	bound, _, err := model.ResolveBillingReport(ctx, period.StartTimestamp, period.EndTimestamp-1, "")
	require.NoError(t, err)
	details, err := model.GetUpstreamBillingDetails(bound, model.UpstreamBillingDetailFilter{Start: period.StartTimestamp, End: period.EndTimestamp - 1, ChannelIds: []int{channel}}, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 4)
	for _, row := range details.Items {
		require.NotNil(t, row.FrozenReplay)
		switch row.RequestId {
		case "charged-image":
			require.NotNil(t, row.BillableImageCount)
			assert.EqualValues(t, 2, *row.BillableImageCount)
			require.NotNil(t, row.FrozenReplay.ExpectedQuota)
			assert.EqualValues(t, 40000, *row.FrozenReplay.ExpectedQuota)
		case "charged-token":
			assert.Nil(t, row.BillableImageCount)
			require.NotNil(t, row.FrozenReplay.ExpectedQuota)
			assert.EqualValues(t, 50, *row.FrozenReplay.ExpectedQuota)
			require.NotNil(t, row.FrozenReplay.DeltaQuota)
			assert.EqualValues(t, 40, *row.FrozenReplay.DeltaQuota)
			assert.Equal(t, model.BillingOriginalReplayMismatch, row.FrozenReplay.Reason)
			assert.Equal(t, map[string]float64{"p": 8, "c": 1}, row.FrozenReplay.TokenInputs)
			cells := frozenBillingDetailCSVCells(row)
			assert.Contains(t, cells, strconv.FormatInt(40, 10))
		case "missing":
			assert.True(t, row.BillableImageMissing)
			assert.Equal(t, model.BillingOriginalReplayMissingUsage, row.FrozenReplay.Reason)
		case "test-image":
			assert.Nil(t, row.FrozenReplay.BookedQuota)
		default:
			t.Fatalf("unexpected detail request %q", row.RequestId)
		}
	}
	var quota int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("request_id = ?", "charged-token").Select("quota").Scan(&quota).Error)
	assert.EqualValues(t, 90, quota)
}
