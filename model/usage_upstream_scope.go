package model

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// UsageUpstreamScope selects leaves of a published usage report.
type UsageUpstreamScope struct {
	URLKey    string
	ChannelID *int
}

func (s UsageUpstreamScope) Valid() bool {
	if s.ChannelID != nil && *s.ChannelID < 0 {
		return false
	}
	if s.URLKey == "" {
		return true
	}
	if len(s.URLKey) > 2048 {
		return false
	}
	if id, ok := strings.CutPrefix(s.URLKey, billingURLGroupChannelFallbackPrefix); ok {
		channel, err := strconv.Atoi(id)
		return err == nil && channel >= 0 && strconv.Itoa(channel) == id
	}
	return normalizeBillingURLGroupKey(s.URLKey) == s.URLKey
}

func (s UsageUpstreamScope) matches(url string, channel int) bool {
	return (s.URLKey == "" || s.URLKey == url) && (s.ChannelID == nil || *s.ChannelID == channel)
}

func (a *usageMetricsAcc) finalizeUpstream() UsageAnalyticsMetrics {
	result := a.finalize()
	if a.originalIncomplete && a.originalKnown && len(a.coefficients) > 0 {
		result.KnownReferenceAmount = billingStatementOriginalQuota(a.reference)
	}
	result.ReferencePending = a.originalIncomplete ||
		(a.originalKnown && !result.ReferenceKnown)
	result.ReferenceCoefficients = slices.Sorted(maps.Keys(a.coefficients))
	return result
}
