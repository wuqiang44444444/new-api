package service

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeContextMigrationRequiresVerifiedShapeAndPair(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })

	const verifiedModel = "claude-opus-4-8"
	header, _ := common.Marshal(map[string]string{"anthropic-beta": strings.Join(claudeContextBetas, ",")})
	sourceMapping := `{"claude-opus-4-8":"provider-opus"}`
	for _, channel := range []model.Channel{
		{Id: 9086, Type: constant.ChannelTypeAnthropic, Key: "source", Status: common.ChannelStatusEnabled, Models: verifiedModel, Group: "default", ModelMapping: &sourceMapping},
		{Id: 9231, Type: constant.ChannelTypeAnthropic, Key: "target", Status: common.ChannelStatusEnabled, Models: verifiedModel, Group: "default", HeaderOverride: common.GetPointer(string(header))},
	} {
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, channel.AddAbilities(db))
	}
	approve := func() {
		require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
			"channel_affinity_setting.failover_verified_pairs": `[{"source_channel_id":9086,"target_channel_id":9231,"model":"` + verifiedModel + `","source_upstream_model":"provider-opus"}]`,
		}))
	}
	approve()
	pairs := model.CurrentRequestPolicy().Affinity.FailoverVerifiedPairs
	require.Len(t, pairs, 1)
	assert.NotEmpty(t, pairs[0].SourceRevision, "saving binds the pair to the approved channel configuration")
	assert.NotEmpty(t, pairs[0].TargetRevision)

	const keepAll = `"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`
	for _, tc := range []struct {
		name, field, model, input string
		want                      bool
	}{
		{"keep all", keepAll, verifiedModel, `"hello"`, true},
		{"unverified model", keepAll, "claude-sonnet-4-6", `"hello"`, false},
		{"unknown operation", strings.Replace(keepAll, "clear_thinking_20251015", "unknown", 1), verifiedModel, `"hello"`, false},
		{"clearing history", strings.Replace(keepAll, `"all"`, `{"type":"thinking_turns","value":1}`, 1), verifiedModel, `"hello"`, false},
		{"duplicate edit property", strings.Replace(keepAll, `"keep":"all"`, `"keep":"all","keep":"all"`, 1), verifiedModel, `"hello"`, false},
		{"duplicate top-level property", keepAll + "," + keepAll, verifiedModel, `"hello"`, false},
		{"signed history", keepAll, verifiedModel, `[{"type":"thinking","thinking":"fixture","signature":"opaque"}]`, false},
		{"prompt cache directive", keepAll, verifiedModel, `[{"type":"text","text":"fixture","cache_control":{"type":"ephemeral"}}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"` + tc.model + `","stream":true,"messages":[{"role":"user","content":` + tc.input + `}],` + tc.field + `}`
			param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/messages?beta=true", tc.model, "default", body)
			param.Ctx.Request.Header.Set("User-Agent", "claude-cli/test")
			param.Ctx.Request.Header.Set("Anthropic-Beta", strings.Join(claudeContextBetas, ",")+","+claudeContextUnsupportedBeta)
			content := inspectFailoverContent(param.Ctx, "messages")
			assert.Equal(t, tc.want, content.claudeKeepAll)
			assert.Equal(t, tc.want, content.kind == failoverText)
		})
	}

	req := &channelFailoverRequest{operation: "messages", modelName: verifiedModel, content: failoverRequestContent{kind: failoverText, claudeKeepAll: true}}
	c := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/messages", verifiedModel, "default", `{}`).Ctx
	load := func(id int) *model.Channel {
		channel, err := model.GetChannelById(id, true)
		require.NoError(t, err)
		return channel
	}
	assert.True(t, req.canMigrateContent(c, load(9086), load(9231)))
	assert.False(t, req.canMigrateContent(c, load(9231), load(9086)), "reverse direction has no evidence")
	assert.True(t, req.isVerifiedClaudeContextSource(load(9086)))

	setChannelAffinityContext(c, channelAffinityMeta{ParamTemplate: map[string]any{"operations": []map[string]any{{"mode": "pass_headers", "value": []string{"Anthropic-Beta"}}}}})
	assert.True(t, req.canMigrateContent(c, load(9086), load(9231)), "header pass-through keeps the verified body")
	setChannelAffinityContext(c, channelAffinityMeta{ParamTemplate: map[string]any{"operations": []map[string]any{{"mode": "set", "path": "context_management", "value": nil}}}})
	assert.False(t, req.canMigrateContent(c, load(9086), load(9231)), "a rule template that rewrites the body is unverified")
	setChannelAffinityContext(c, channelAffinityMeta{})

	t.Run("bound session only bypasses to a healthy verified target", func(t *testing.T) {
		require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
			"channel_affinity_setting.enabled":              "true",
			"channel_affinity_setting.failover_mode":        "enabled",
			"channel_affinity_setting.failover_channel_ids": "[9086,9231]",
			"channel_affinity_setting.rules":                `[{"name":"claude cli trace","model_regex":["^claude-"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":"strict","failover_on_unavailable":true,"include_rule_name":true},{"name":"codex cli trace"}]`,
		}))
		request := func() *RetryParam {
			body := `{"model":"` + verifiedModel + `","stream":true,"messages":[{"role":"user","content":"hello"}],` + keepAll + `}`
			param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/messages?beta=true", verifiedModel, "default", body)
			param.Ctx.Request.Header.Set("User-Agent", "claude-cli/test")
			param.Ctx.Request.Header.Set("Anthropic-Beta", strings.Join(claudeContextBetas, ",")+","+claudeContextUnsupportedBeta)
			param.Ctx.Request.Header.Set("X-Session", "verified-pair-failover")
			return param
		}
		model.InitChannelCache()
		seed := request()
		GetPreferredChannelByAffinity(seed.Ctx, verifiedModel, "default")
		RecordChannelAffinity(seed.Ctx, 9086)
		setCooldown := func(channel *model.Channel, state string) {
			_, _, err := updateChannelFailoverHealth(channelFailoverScope(channel, verifiedModel, "messages", -1), func(next *ChannelFailoverHealth, now time.Time) bool {
				next.State, next.NextCheck = state, now.Add(time.Minute)
				return true
			})
			require.NoError(t, err)
		}
		selectBound := func(want int) *RetryParam {
			param := request()
			channel, _, err := SelectChannelForRequest(param.Ctx, verifiedModel, param)
			require.Nil(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, want, channel.Id)
			return param
		}
		source, target := load(9086), load(9231)
		setCooldown(source, "cooldown")
		selectBound(9231)

		setCooldown(target, "cooldown")
		param := selectBound(9086)
		// A subsequent same-request supplement cannot use the cooling target either.
		state := failoverRequestFrom(param.Ctx)
		state.failed = map[int]bool{source.Id: true}
		state.failure, state.supplement = source, true
		param.SetRetry(1)
		backup, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		assert.Nil(t, backup)

		setCooldown(target, "")
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9231).Update("base_url", "https://unverified.invalid").Error)
		model.InitChannelCache()
		selectBound(9086)
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9231).Update("base_url", target.BaseURL).Error)
		model.InitChannelCache()
		selectBound(9231)
	})

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9231).Update("base_url", "https://changed.invalid").Error)
	assert.False(t, req.canMigrateContent(c, load(9086), load(9231)), "a target configuration change invalidates the approval")
	approve()
	assert.True(t, req.canMigrateContent(c, load(9086), load(9231)), "re-approval binds the current configuration")

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9231).Update("model_mapping", `{"`+verifiedModel+`":"claude-sonnet-4-6"}`).Error)
	approve()
	assert.False(t, req.canMigrateContent(c, load(9086), load(9231)), "the target must send the verified upstream model")
}
