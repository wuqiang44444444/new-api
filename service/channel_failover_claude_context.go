package service

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const claudeContextUnsupportedBeta = "prompt-caching-scope-2026-01-05"

var claudeContextBetas = []string{
	"claude-code-20250219", "interleaved-thinking-2025-05-14", "thinking-token-count-2026-05-13",
	"context-management-2025-06-27", "mid-conversation-system-2026-04-07", "effort-2025-11-24",
}

func claudeKeepAllContext(value gjson.Result) bool {
	if !value.IsObject() || len(value.Map()) != 1 || !uniqueClaudeContextFields(value) {
		return false
	}
	edits := value.Get("edits")
	if !edits.IsArray() {
		return false
	}
	items := edits.Array()
	if len(items) != 1 {
		return false
	}
	edit := items[0]
	return edit.IsObject() && len(edit.Map()) == 2 && uniqueClaudeContextFields(edit) &&
		edit.Get("type").Str == "clear_thinking_20251015" && edit.Get("keep").Str == "all"
}

func uniqueClaudeContextFields(value gjson.Result) bool {
	seen := make(map[string]bool)
	unique := true
	value.ForEach(func(key, _ gjson.Result) bool {
		unique = !seen[key.Str]
		seen[key.Str] = true
		return unique
	})
	return unique
}

// The verified CLI profile carries no cache directives or stored history.
// Keep the original JSON and use the existing channel header override for the
// provider's beta negotiation.
func claudeContextProfile(c *gin.Context, request gjson.Result) bool {
	if !strings.HasPrefix(c.Request.UserAgent(), "claude-cli/") ||
		c.Query("beta") != "true" || request.Get("stream").Type != gjson.True ||
		!claudeContextModelVerified(request.Get("model").Str) || !uniqueClaudeContextFields(request) ||
		request.Get("prompt_cache_key").Exists() || request.Get("prompt_cache_retention").Exists() ||
		!claudeContextBetaSet(strings.Join(c.Request.Header.Values("Anthropic-Beta"), ","), true) {
		return false
	}
	if tools := request.Get("tools"); tools.Exists() && (!tools.IsArray() || len(tools.Array()) != 0) {
		return false
	}
	messages := request.Get("messages")
	return messages.IsArray() && affinityArrayEvery(messages, func(message gjson.Result) bool {
		role := message.Get("role").Str
		return (role == "user" || role == "assistant") && uniqueClaudeContextFields(message)
	}) && !claudeContextHasCacheControl(request)
}

func claudeContextModelVerified(modelName string) bool {
	return modelName != "" && slices.ContainsFunc(model.CurrentRequestPolicy().Affinity.FailoverVerifiedPairs,
		func(pair operation_setting.ChannelFailoverVerifiedPair) bool { return pair.Model == modelName })
}

func claudeContextHasCacheControl(value gjson.Result) bool {
	found := false
	if value.IsObject() || value.IsArray() {
		value.ForEach(func(key, item gjson.Result) bool {
			found = strings.EqualFold(key.Str, "cache_control") || claudeContextHasCacheControl(item)
			return !found
		})
	}
	return found
}

func claudeContextBetaSet(raw string, allowClientCacheBeta bool) bool {
	betas := strings.Split(raw, ",")
	for i := range betas {
		betas[i] = strings.TrimSpace(betas[i])
	}
	if allowClientCacheBeta && slices.Contains(betas, claudeContextUnsupportedBeta) {
		if len(betas) != len(claudeContextBetas)+1 {
			return false
		}
		betas = slices.DeleteFunc(betas, func(beta string) bool { return beta == claudeContextUnsupportedBeta })
	}
	slices.Sort(betas)
	expected := slices.Clone(claudeContextBetas)
	slices.Sort(expected)
	return slices.Equal(betas, expected)
}

func (r *channelFailoverRequest) canMigrateContent(c *gin.Context, source, target *model.Channel) bool {
	if !canMigrateChannelContent(r.operation, r.content.kind, source, target) {
		return false
	}
	if !r.content.claudeKeepAll {
		return true
	}
	if !claudeContextTemplateSafe(c) {
		return false
	}
	for _, pair := range model.CurrentRequestPolicy().Affinity.FailoverVerifiedPairs {
		if pair.Model != r.modelName {
			continue
		}
		// A recovery trial returns only to a verified source of this profile.
		if source == nil && verifiedClaudeContextSide(target, pair.SourceChannelID, pair.SourceRevision, pair) {
			return true
		}
		if source != nil && verifiedClaudeContextSide(source, pair.SourceChannelID, pair.SourceRevision, pair) &&
			verifiedClaudeContextSide(target, pair.TargetChannelID, pair.TargetRevision, pair) && claudeContextBetaOverride(target) {
			return true
		}
	}
	return false
}

func (r *channelFailoverRequest) isVerifiedClaudeContextSource(channel *model.Channel) bool {
	for _, pair := range model.CurrentRequestPolicy().Affinity.FailoverVerifiedPairs {
		if pair.Model == r.modelName && verifiedClaudeContextSide(channel, pair.SourceChannelID, pair.SourceRevision, pair) {
			return true
		}
	}
	return false
}

// verifiedClaudeContextSide binds the evidence to the channel configuration
// and to the upstream model that the channel mapping actually sends.
func verifiedClaudeContextSide(channel *model.Channel, id int, revision string, pair operation_setting.ChannelFailoverVerifiedPair) bool {
	upstreamModel := pair.EffectiveUpstreamModel()
	if id == pair.SourceChannelID && pair.SourceUpstreamModel != "" {
		upstreamModel = pair.SourceUpstreamModel
	}
	return channel != nil && channel.Id == id && revision != "" && model.ChannelAlertRevision(channel) == revision &&
		claudeContextChannelSafe(channel) && mappedUpstreamModel(channel, pair.Model) == upstreamModel
}

func claudeContextBetaOverride(target *model.Channel) bool {
	headers := target.GetHeaderOverride()
	if len(headers) != 1 {
		return false
	}
	for key, value := range headers {
		betas, ok := value.(string)
		if strings.EqualFold(key, "anthropic-beta") && ok && claudeContextBetaSet(betas, false) {
			return true
		}
	}
	return false
}

// mappedUpstreamModel follows the channel model_mapping chain. Reasoning
// suffix fallbacks are not resolved here, so such models never match.
func mappedUpstreamModel(channel *model.Channel, customerModel string) string {
	if reasoning.BaseModelName(customerModel) != customerModel {
		return ""
	}
	mapping := make(map[string]string)
	if raw := channel.GetModelMapping(); raw != "" && raw != "{}" && common.UnmarshalJsonStr(raw, &mapping) != nil {
		return ""
	}
	current := customerModel
	visited := map[string]bool{current: true}
	for {
		next := mapping[current]
		if next == "" || next == current {
			return current
		}
		if visited[next] || reasoning.BaseModelName(next) != next {
			return ""
		}
		visited[next] = true
		current = next
	}
}

// Only header pass-through templates keep the verified request body. The
// target's header override still replaces the forwarded beta header.
func claudeContextTemplateSafe(c *gin.Context) bool {
	meta, ok := getChannelAffinityMeta(c)
	if !ok || len(meta.ParamTemplate) == 0 {
		return true
	}
	raw, err := common.Marshal(meta.ParamTemplate)
	if err != nil {
		return false
	}
	template := gjson.ParseBytes(raw)
	operations := template.Get("operations")
	return len(template.Map()) == 1 && operations.IsArray() && affinityArrayEvery(operations, func(operation gjson.Result) bool {
		return operation.Get("mode").Str == "pass_headers"
	})
}

func claudeContextChannelSafe(channel *model.Channel) bool {
	if channel.Type != constant.ChannelTypeAnthropic || channel.ChannelInfo.IsMultiKey ||
		model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return false
	}
	if raw := channel.ParamOverride; raw != nil && *raw != "" {
		var overrides map[string]any
		if common.UnmarshalJsonStr(*raw, &overrides) != nil || len(overrides) != 0 {
			return false
		}
	}
	return true
}
