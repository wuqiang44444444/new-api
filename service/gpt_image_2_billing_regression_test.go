package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTImage2SynchronousBillingSnapshotDoesNotCreateTask(t *testing.T) {
	truncate(t)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	const (
		userID       = 820
		tokenID      = 821
		channelID    = 822
		starting     = 1_000_000
		modelPrice   = 0.04
		groupRatio   = 0.8
		imageCount   = 2
		expectedCost = 32_000
	)
	seedUser(t, userID, starting)
	seedToken(t, tokenID, userID, "gpt-image-2-token", starting)
	seedChannel(t, channelID)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Set("token_name", "gpt-image-2-regression")
	c.Set("username", "test_user")
	c.Set(common.RequestIdKey, "gpt-image-2-billing-snapshot")

	startedAt := time.Unix(1_700_000_000, 0)
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		UserQuota:       starting,
		TokenId:         tokenID,
		TokenKey:        "gpt-image-2-token",
		TokenGroup:      "default",
		UsingGroup:      "default",
		UserGroup:       "default",
		OriginModelName: "gpt-image-2",
		StartTime:       startedAt,
		FirstResponseTime: startedAt.Add(
			100 * time.Millisecond,
		),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:   channelID,
			ChannelType: constant.ChannelTypeOpenAI,
		},
		PriceData: types.PriceData{
			ModelPrice: modelPrice,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: groupRatio,
			},
			QuotaToPreConsume: expectedCost,
		},
	}
	info.PriceData.AddOtherRatio("n", imageCount)

	require.Nil(t, PreConsumeBilling(c, expectedCost, info))
	assert.Equal(t, expectedCost, info.FinalPreConsumedQuota)
	assert.Equal(t, starting-expectedCost, getUserQuota(t, userID))
	assert.Equal(t, starting-expectedCost, getTokenRemainQuota(t, tokenID))

	PostTextConsumeQuota(c, info, &dto.Usage{
		PromptTokens: 1,
		TotalTokens:  1,
	}, []string{"大小 1024x1024", "生成数量 2"})

	assert.Equal(t, starting-expectedCost, getUserQuota(t, userID))
	assert.Equal(t, starting-expectedCost, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, expectedCost, getTokenUsedQuota(t, tokenID))
	assert.Equal(t, float64(imageCount), info.PriceData.OtherRatios()["n"])
	assert.False(t, info.BillingTransferredToTask)
	assert.Nil(t, info.TaskRelayInfo)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, "gpt-image-2", log.ModelName)
	assert.Equal(t, expectedCost, log.Quota)
	assert.Equal(t, 1, log.PromptTokens)
	assert.Zero(t, log.CompletionTokens)
	assert.Equal(t, channelID, log.ChannelId)
	assert.Equal(t, tokenID, log.TokenId)
	assert.Equal(t, "default", log.Group)
	assert.Contains(t, log.Content, "生成数量 2")

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, modelPrice, other["model_price"])
	assert.Equal(t, groupRatio, other["group_ratio"])
	assert.Equal(t, "wallet", other["billing_source"])
	assert.Equal(t, "/v1/images/generations", other["request_path"])
}
