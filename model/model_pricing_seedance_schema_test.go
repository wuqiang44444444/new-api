package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seedanceAttributionPluginSource = `
export const meta = {
  apiVersion: 1, key: "attribution-usage-probe", name: "Attribution Usage Probe", version: "1.0.0",
  author: {name: "Test"}, models: ["native-usage-model"], fetchMode: "per_task",
  usageSchema: {tokens: {type: "number", unit: "token"}},
  usageExamples: [{label: "100k tokens", facts: {tokens: 100000}}]
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`

func insertAttributionChannel(t *testing.T, channelID, channelType int, models string, settings dto.ChannelOtherSettings, status int) {
	t.Helper()
	channel := &Channel{
		Id:     channelID,
		Type:   channelType,
		Key:    fmt.Sprintf("key-%d", channelID),
		Status: status,
		Name:   fmt.Sprintf("channel-%d", channelID),
		Models: models,
	}
	channel.SetOtherSettings(settings)
	require.NoError(t, DB.Create(channel).Error)
}

// TestModelPricingSnapshotAttributionForSeedance 保护管理价格接口的 Seedance 归属：
// 类型化渠道客户模型不得被通用插件模型索引赋予原生 usage_schema（本次故障根因），
// 禁用渠道仍参与归属，跨类型同名合同冲突一致报告。
func TestModelPricingSnapshotAttributionForSeedance(t *testing.T) {
	resetPricingEndpointTestTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	_, err := jsplugin.DefaultRegistry.Register(seedanceAttributionPluginSource, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("attribution-usage-probe") })

	insertAttributionChannel(t, 931, constant.ChannelTypeSeedanceLink, "seedance-model,seedance-conflict",
		dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1}, common.ChannelStatusEnabled)
	insertAttributionChannel(t, 932, constant.ChannelTypeSeedanceLink, "seedance-disabled",
		dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine}, common.ChannelStatusManuallyDisabled)
	insertAttributionChannel(t, 933, constant.ChannelTypeTaskPlugin, "native-usage-model", dto.ChannelOtherSettings{}, common.ChannelStatusEnabled)
	insertAttributionChannel(t, 934, constant.ChannelTypeDoubaoVideo, "seedance-conflict", dto.ChannelOtherSettings{}, common.ChannelStatusEnabled)

	snapshot, err := GetModelPricingSnapshot([]string{"seedance-model", "seedance-disabled", "native-usage-model", "seedance-conflict", "plain-model"})
	require.NoError(t, err)
	entries := make(map[string]ModelPricingEntry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ModelName] = entry
	}

	t.Run("seedance model gets its own contract, not the native schema", func(t *testing.T) {
		entry := entries["seedance-model"]
		assert.True(t, entry.PreconsumeTokenBudget)
		assert.False(t, entry.BillingContractConflict)
		assert.Contains(t, entry.UsageSchema, "tokens")
		assert.Contains(t, entry.UsageSchema, "ratio", "feicai protocol extras stay declared")
		assert.Equal(t, "token", entry.UsageSchema["tokens"].Unit)
	})

	t.Run("disabled link channels still own their customer model", func(t *testing.T) {
		entry := entries["seedance-disabled"]
		assert.True(t, entry.PreconsumeTokenBudget)
		assert.Contains(t, entry.UsageSchema, "tokens")
		assert.NotContains(t, entry.UsageSchema, "ratio")
	})

	t.Run("native plugin models keep the native schema", func(t *testing.T) {
		entry := entries["native-usage-model"]
		assert.False(t, entry.PreconsumeTokenBudget)
		assert.False(t, entry.BillingContractConflict)
		assert.Contains(t, entry.UsageSchema, "tokens")
		assert.Len(t, entry.UsageSchema, 1)
	})

	t.Run("cross-type name conflicts are reported on the admin side", func(t *testing.T) {
		entry := entries["seedance-conflict"]
		assert.True(t, entry.PreconsumeTokenBudget)
		assert.True(t, entry.BillingContractConflict)
	})

	t.Run("plain models carry no attribution", func(t *testing.T) {
		entry := entries["plain-model"]
		assert.False(t, entry.PreconsumeTokenBudget)
		assert.False(t, entry.BillingContractConflict)
		assert.Empty(t, entry.UsageSchema)
	})
}

func TestSeedancePricingSnapshotMatchesSaveContractAcrossChannelStates(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			require.NoError(t, DB.AutoMigrate(&Option{}))
			status := common.ChannelStatusManuallyDisabled
			if active {
				status = common.ChannelStatusEnabled
			}
			insertAttributionChannel(t, 941, constant.ChannelTypeSeedanceLink, "shared-price", dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine}, common.ChannelStatusManuallyDisabled)
			insertAttributionChannel(t, 942, constant.ChannelTypeSeedanceLink, "shared-price", dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1}, status)
			snapshot, err := GetModelPricingSnapshot([]string{"shared-price"})
			require.NoError(t, err)
			require.Len(t, snapshot.Entries, 1)
			_, offered := snapshot.Entries[0].UsageSchema["ratio"]
			assert.Equal(t, active, offered)
			handled, err := ValidateSeedanceBillingExpression("shared-price", `u("ratio") == "21:9" ? tier("wide", 0.8) : tier("base", 0.4)`)
			require.True(t, handled)
			if active {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "ratio")
			}
		})
	}
}
