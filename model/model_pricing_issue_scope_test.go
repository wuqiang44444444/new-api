package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPricingIssuesRequireEligibleNativeChannels(t *testing.T) {
	resetPricingEndpointTestTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	_, ok := jsplugin.DefaultRegistry.Generation().Get("doubao")
	require.True(t, ok, "the real Doubao plugin supplies native channel eligibility")

	const native = "doubao-seedance-2-0-fast-260128"
	const aliasTarget = "doubao-seedance-2-0-mini-260615"
	const invalid = "doubao-seedance-2-5-260628"
	malformed := common.GetPointer(`{"task_plugin_key":"doubao"`)
	channels := []Channel{
		{Id: 951, Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Models: "link-only",
			ModelMapping: common.GetPointer(`{"link-only":"doubao-seedance-2-0-260128"}`)},
		{Id: 952, Type: constant.ChannelTypeAsyncImage, Status: common.ChannelStatusEnabled, Models: "relay-image",
			ModelMapping: common.GetPointer(`{"relay-image":"doubao-seedream-5-0-260128"}`)},
		{Id: 953, Type: constant.ChannelTypeVolcEngine, Status: common.ChannelStatusEnabled, Models: native + ",unmapped-alias"},
		{Id: 954, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Models: invalid, Setting: malformed},
		{Id: 955, Type: constant.ChannelTypeVolcEngine, Status: common.ChannelStatusManuallyDisabled, Models: "doubao-seedream-5-0-pro-260628"},
		{Id: 956, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "unmapped-alias",
			ModelMapping: common.GetPointer(`{"unmapped-alias":"doubao-seedance-2-0-mini-260615"}`)},
		{Id: 957, Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusEnabled, Models: "minimax-link-only",
			ModelMapping: common.GetPointer(`{"minimax-link-only":"doubao-seedance-2-5-260628"}`)},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine})
	require.NoError(t, DB.Create(&channels).Error)
	for _, ability := range []Ability{
		{ChannelId: 952, Model: "relay-image"},
		{ChannelId: 953, Model: native},
		{ChannelId: 953, Model: "unmapped-alias"},
		{ChannelId: 954, Model: invalid},
		{ChannelId: 955, Model: "doubao-seedream-5-0-pro-260628"},
		{ChannelId: 959, Model: "doubao-seedream-5-0-lite-260128"},
	} {
		insertPricingEndpointAbility(t, ability.ChannelId, ability.Model)
	}
	InitChannelCache()

	generation := jsplugin.DefaultRegistry.Generation()
	for _, name := range []string{"link-only", "minimax-link-only"} {
		_, found := ResolveTaskModelAlias(generation, name)
		assert.False(t, found, "typed Link channels must not register native aliases")
	}
	issues, err := GetModelPricingIssues("doubao")
	require.NoError(t, err)
	assert.Equal(t, []ModelPricingIssue{{
		ModelName: native, PluginKey: "doubao", PluginName: "Doubao", Reason: ModelPricingIssueUnconfigured,
	}, {
		ModelName: aliasTarget, PluginKey: "doubao", PluginName: "Doubao", Reason: ModelPricingIssueUnconfigured,
	}}, issues, "an eligible channel can serve a globally registered native alias without owning the mapping")

	var actual Channel
	require.NoError(t, DB.Select("setting").Take(&actual, 954).Error)
	assert.Equal(t, malformed, actual.Setting, "reading price alerts must preserve malformed settings")
}
