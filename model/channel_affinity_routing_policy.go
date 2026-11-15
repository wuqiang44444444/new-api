package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Struct slices decode into a fresh slice: decoding into the live slice would
// keep omitted fields, such as a disabled failover_on_unavailable, from earlier saves.
func replaceChannelAffinityRules(cfg any, configKey, value string) bool {
	affinity, ok := cfg.(*operation_setting.ChannelAffinitySetting)
	if !ok {
		return false
	}
	switch configKey {
	case "rules":
		var rules []operation_setting.ChannelAffinityRule
		if common.UnmarshalJsonStr(value, &rules) == nil {
			affinity.Rules = rules
		}
	case "failover_verified_pairs":
		var pairs []operation_setting.ChannelFailoverVerifiedPair
		if common.UnmarshalJsonStr(value, &pairs) == nil {
			affinity.FailoverVerifiedPairs = pairs
		}
	default:
		return false
	}
	return true
}

// stampFailoverPairRevisions binds each newly saved pair to the channels'
// current configuration. Operators clear a revision to re-approve a pair.
func stampFailoverPairRevisions(values map[string]string) error {
	const key = "channel_affinity_setting.failover_verified_pairs"
	raw, ok := values[key]
	if !ok {
		return nil
	}
	var pairs []operation_setting.ChannelFailoverVerifiedPair
	if err := common.UnmarshalJsonStr(raw, &pairs); err != nil {
		return fmt.Errorf("invalid verified failover pairs")
	}
	for i := range pairs {
		for _, side := range []struct {
			id       int
			revision *string
		}{{pairs[i].SourceChannelID, &pairs[i].SourceRevision}, {pairs[i].TargetChannelID, &pairs[i].TargetRevision}} {
			if *side.revision != "" || side.id <= 0 {
				continue
			}
			channel, err := GetChannelById(side.id, true)
			if err != nil {
				return fmt.Errorf("verified failover pair channel %d not found", side.id)
			}
			*side.revision = ChannelAlertRevision(channel)
		}
	}
	if pairs == nil {
		pairs = []operation_setting.ChannelFailoverVerifiedPair{}
	}
	encoded, err := common.Marshal(pairs)
	if err != nil {
		return err
	}
	values[key] = string(encoded)
	return nil
}

func validateAffinityRoutingPolicy(setting operation_setting.ChannelAffinitySetting) error {
	switch setting.FailoverMode {
	case "", "off", "observe":
	case "enabled":
		if len(setting.FailoverChannelIDs) == 0 {
			return fmt.Errorf("channel failover requires rollout channel IDs")
		}
	default:
		return fmt.Errorf("invalid channel failover mode")
	}
	for _, id := range setting.FailoverChannelIDs {
		if id <= 0 {
			return fmt.Errorf("invalid channel failover channel ID")
		}
	}

	seen := make(map[operation_setting.ChannelFailoverVerifiedPair]bool, len(setting.FailoverVerifiedPairs))
	for _, pair := range setting.FailoverVerifiedPairs {
		if pair.SourceChannelID <= 0 || pair.TargetChannelID <= 0 || pair.SourceChannelID == pair.TargetChannelID ||
			strings.TrimSpace(pair.Model) == "" || pair.Model != strings.TrimSpace(pair.Model) ||
			pair.UpstreamModel != strings.TrimSpace(pair.UpstreamModel) ||
			pair.SourceUpstreamModel != strings.TrimSpace(pair.SourceUpstreamModel) {
			return fmt.Errorf("invalid verified failover pair")
		}
		identity := operation_setting.ChannelFailoverVerifiedPair{SourceChannelID: pair.SourceChannelID, TargetChannelID: pair.TargetChannelID, Model: pair.Model}
		if seen[identity] {
			return fmt.Errorf("duplicate verified failover pair")
		}
		seen[identity] = true
	}

	// Historical option sets can lack a built-in rule. Protect rules that
	// exist at save time without making those deployments unable to edit.
	if setting.LayeredAffinity {
		names := make(map[string]bool, len(setting.Rules))
		for _, rule := range setting.Rules {
			name := strings.TrimSpace(rule.Name)
			if name == "" || names[name] {
				return fmt.Errorf("affinity rules require distinct names")
			}
			names[name] = true
		}
	}
	return nil
}

func protectBuiltInAffinityRules(previous, next *RequestPolicySnapshot) error {
	for _, rule := range previous.Affinity.Rules {
		if rule.Name != "claude cli trace" && rule.Name != "codex cli trace" {
			continue
		}
		found := false
		for _, updated := range next.Affinity.Rules {
			if updated.Name == rule.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("system affinity rule %q cannot be deleted", rule.Name)
		}
	}
	return nil
}
