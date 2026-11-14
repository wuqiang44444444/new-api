package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

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
