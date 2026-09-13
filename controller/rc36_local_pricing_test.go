package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRC36ModelPricingPreservesSeedanceBudgetAndProjection(t *testing.T) {
	modelManagementDB(t, "sqlite", "")
	channel := model.Channel{Type: constant.ChannelTypeSeedanceLink, Models: "sync-video", Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine})
	require.NoError(t, model.DB.Create(&channel).Error)
	snapshot, err := model.GetModelPricingSnapshot([]string{"sync-video"})
	require.NoError(t, err)
	change := model.ModelPricingChange{ModelName: "sync-video", ExpectedVersion: snapshot.Entries[0].Version, Pricing: model.PricingValues{
		"billing_setting.billing_mode":             "tiered_expr",
		"billing_setting.billing_expr":             `tier("base", u("tokens") * 5 / 1000000)`,
		billing_setting.TaskPreConsumeTokensOption: float64(325000),
	}}
	require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{change}))
	snapshot, err = model.GetModelPricingSnapshot([]string{"sync-video"})
	require.NoError(t, err)
	assert.Equal(t, float64(325000), snapshot.Entries[0].Configured[billing_setting.TaskPreConsumeTokensOption])
	tokens, ok := billing_setting.GetTaskPreConsumeTokens("sync-video")
	assert.True(t, ok)
	assert.Equal(t, 325000, tokens)
	service.AttachModelPricingBillingDisplay(snapshot)
	require.NotNil(t, snapshot.Entries[0].BillingDisplay)
	// u() 任务表达式由任务计价展示路径渲染；token 投影按设计返回 opaque，
	// 不得生成猜测的单价展开。
	assert.Equal(t, "opaque", string(snapshot.Entries[0].BillingDisplay.Status))
	assert.Equal(t, billingexpr.DisplayReasonTaskUsageExpression, snapshot.Entries[0].BillingDisplay.Reason)
	change.ExpectedVersion = snapshot.Entries[0].Version
	for _, invalid := range []float64{0, -1, 0.5, float64(billing_setting.MaxTaskPreConsumeTokens) + 1} {
		change.Pricing[billing_setting.TaskPreConsumeTokensOption] = invalid
		require.Error(t, model.UpdateModelPricing([]model.ModelPricingChange{change}))
		unchanged, err := model.GetModelPricingSnapshot([]string{"sync-video"})
		require.NoError(t, err)
		assert.Equal(t, snapshot.Entries[0].Version, unchanged.Entries[0].Version)
	}
}
