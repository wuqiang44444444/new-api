package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSummaryPagesKeepChannelsIndependentAndDoNotEmbedModels(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	for id := 1; id <= 3; id++ {
		base := "https://same.example"
		require.NoError(t, db.Create(&Channel{Id: id, Name: fmt.Sprintf("channel-%d", id), BaseURL: &base}).Error)
		for _, name := range []string{"a", "b", "c"} {
			require.NoError(t, db.Create(&Log{CreatedAt: 1100, Type: LogTypeConsume, ChannelId: id, ModelName: name, TokenName: "模型测试", Content: "模型测试", Quota: 10, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":10}}`}).Error)
		}
	}
	filter := UpstreamSummaryPageFilter{Level: "channels", Page: 1, PageSize: 1}
	first, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, first.Channels, 1)
	assert.Equal(t, 3, first.Total)
	assert.EqualValues(t, 9, first.ModelCount)
	assert.Empty(t, first.Channels[0].Models)
	require.NotNil(t, first.Channels[0].OriginalAmount)
	assert.EqualValues(t, 30, *first.Channels[0].OriginalAmount)
	filter.Page = 2
	second, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, second.Channels, 1)
	assert.Equal(t, 2, second.Channels[0].ChannelId)
	assert.Equal(t, first.DataQuality, second.DataQuality)
	filter.Level, filter.ChannelID = "models", 2
	models, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	assert.Equal(t, 3, models.Total)
	assert.Equal(t, "b", models.Models[0].ProviderModel)
	filter.Level, filter.Search, filter.ChannelID, filter.Page = "options", "channel-3", 0, 1
	options, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, options.Channels, 1)
	assert.Equal(t, 3, options.Channels[0].ChannelId)
	filter.Page = 2
	empty, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	assert.Empty(t, empty.Channels)
	assert.Equal(t, 1, empty.Total)
}
