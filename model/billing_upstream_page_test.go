package model

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpstreamSummaryPagesKeepTotalsAndDoNotEmbedChildren(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	for id := 1; id <= 3; id++ {
		base := "https://one.example"
		if id == 3 {
			base = "https://two.example"
		}
		require.NoError(t, db.Create(&Channel{Id: id, Name: fmt.Sprintf("channel-%d", id), BaseURL: &base}).Error)
		for _, name := range []string{"a", "b", "c"} {
			require.NoError(t, db.Create(&Log{CreatedAt: 1100, Type: LogTypeConsume, ChannelId: id, ModelName: name, TokenName: "模型测试", Content: "模型测试", Quota: 10, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":10}}`}).Error)
		}
	}
	full, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	filter := UpstreamSummaryPageFilter{Level: "groups", Page: 1, PageSize: 1}
	first, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, first.Groups, 1)
	assert.Equal(t, 2, first.Total)
	assert.Equal(t, full.DataQuality, first.DataQuality)
	assert.Empty(t, first.Groups[0].Channels)
	assert.Empty(t, first.Groups[0].ChannelIds)
	assert.Equal(t, full.Groups[0].OriginalAmount, first.Groups[0].OriginalAmount)
	filter.Page = 2
	second, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, second.Groups, 1)
	assert.NotEqual(t, first.Groups[0].UrlKey, second.Groups[0].UrlKey)
	assert.Equal(t, first.DataQuality, second.DataQuality)
	filter = UpstreamSummaryPageFilter{Level: "channels", URLKey: "https://one.example", Page: 2, PageSize: 1}
	channels, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, channels.Channels, 1)
	assert.Equal(t, 2, channels.Total)
	assert.Equal(t, 2, channels.Channels[0].ChannelId)
	assert.Empty(t, channels.Channels[0].Models)
	filter.Level = "models"
	filter.ChannelID = 2
	models, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	assert.Equal(t, 3, models.Total)
	assert.Equal(t, "b", models.Models[0].ProviderModel)
	assert.Equal(t, full.Groups[0].Channels[1].DataQuality, models.DataQuality)
	filter.URLKey = "https://two.example"
	wrongScope, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	assert.Zero(t, wrongScope.Total)
	filter = UpstreamSummaryPageFilter{Level: "options", Search: "two", Page: 1, PageSize: 1}
	options, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, options.Total)
	require.Len(t, options.Groups, 1)
	assert.Equal(t, "https://two.example", options.Groups[0].UrlKey)
	filter.Page = 2
	empty, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	assert.Empty(t, empty.Groups)
	assert.Equal(t, 1, empty.Total)
}
