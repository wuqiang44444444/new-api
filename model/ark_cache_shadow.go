package model

import (
	"context"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ArkCacheShadowHour struct {
	HourStart            int64 `json:"hour_start"`
	PrefixPeakCandidate  int64 `json:"prefix_peak_candidate"`
	PrefixSumCandidate   int64 `json:"prefix_sum_candidate"`
	SessionPeakCandidate int64 `json:"session_peak_candidate"`
	SessionSumCandidate  int64 `json:"session_sum_candidate"`
	UnverifiedWrites     int   `json:"unverified_writes"`
}

type arkCacheShadowChange struct {
	second  int64
	prefix  int64
	session int64
}

// ArkCacheShadowHours replays measured lifetimes in one frozen local scope.
// These counters are hypotheses for supplier reconciliation and never settle funds.
func ArkCacheShadowHours(ctx context.Context, channelID, userID, appID int, customerModel string, start, end int64) ([]ArkCacheShadowHour, error) {
	const maxTTL = 7 * 24 * 3600
	customerModel = strings.TrimSpace(customerModel)
	if channelID <= 0 || userID <= 0 || appID <= 0 || customerModel == "" || len(customerModel) > 191 ||
		start <= maxTTL+3600 || end <= start || end > common.GetTimestamp() || end-start > maxTTL ||
		start%3600 != 0 || end%3600 != 0 {
		return nil, ErrArkCacheConflict
	}
	var rows []ArkPrefixCache
	err := DB.WithContext(ctx).Scopes(arkCacheExecutionScope).
		Select("id", "endpoint", "credential", "provider_id", "parent_id", "session_round", "state", "sent_at",
			"created_at", "expires_at", "requested_until", "deleted_at", "delete_requested_at", "input_tokens", "output_tokens", "read_tokens").
		Where("channel_id = ? AND user_id = ? AND token_id = ? AND model = ? AND sent_at >= ? AND sent_at < ? AND state <> ?",
			channelID, userID, appID, customerModel, start-maxTTL-3600, end+3600, ArkCacheRejected).
		Order("sent_at, id").Limit(5001).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > 5000 {
		return nil, ErrArkCacheLimit
	}
	hours := make([]ArkCacheShadowHour, (end-start)/3600)
	changes := make([][]arkCacheShadowChange, len(hours))
	for i := range hours {
		hours[i].HourStart = start + 3600*int64(i)
	}
	var scope string
	observedIDs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		knownStart := row.ProviderID != "" && row.CreatedAt > 0 && row.State != ArkCacheSending && row.State != ArkCacheUnknown
		left, right := row.CreatedAt, arkCacheEnd(row)
		unverified := !knownStart
		deletionUncertain := row.State == ArkCacheDeleting || row.State == ArkCacheAbsent
		if !knownStart {
			left, right = row.SentAt, max(row.RequestedUntil, row.ExpiresAt)
			if row.DeletedAt > 0 {
				right = min(right, row.DeletedAt)
			}
		}
		if right <= left || right <= start || left >= end {
			continue
		}
		rowScope := row.Endpoint + "\x00" + row.Credential
		if scope == "" {
			scope = rowScope
		}
		if rowScope != scope {
			return nil, ErrArkCacheConflict
		}
		if knownStart {
			if _, exists := observedIDs[row.ProviderID]; exists {
				return nil, ErrArkCacheConflict
			}
			observedIDs[row.ProviderID] = struct{}{}
		}
		var prefix, session int64
		switch {
		case !knownStart || row.InputTokens <= 0 || row.InputTokens > 1024*1024 ||
			row.ReadTokens < 0 || row.ReadTokens > row.InputTokens || row.OutputTokens < 0 || row.OutputTokens > 1024*1024:
			unverified = true
		case row.SessionRound > 0:
			// Candidate once per newly retained portion. Neither raw input
			// nor prior read hits prove the number of tokens stored.
			session = int64(arkSessionStorageTokens(row))
		case row.ParentID == "":
			prefix = int64(row.InputTokens)
		default:
			// A fixed-prefix reuse can produce a separately stored response.
			unverified = true
		}
		left, right = max(left, start), min(right, end)
		for i := (left - start) / 3600; i <= (right-1-start)/3600; i++ {
			hour := &hours[i]
			if unverified || (deletionUncertain && (row.DeleteRequestedAt == 0 || hour.HourStart+3600 > row.DeleteRequestedAt)) {
				hour.UnverifiedWrites++
			}
			hour.PrefixSumCandidate += prefix
			hour.SessionSumCandidate += session
			if prefix > 0 || session > 0 {
				changes[i] = append(changes[i], arkCacheShadowChange{max(left, hour.HourStart), prefix, session},
					arkCacheShadowChange{min(right, hour.HourStart+3600), -prefix, -session})
			}
		}
	}
	for i, events := range changes {
		sort.Slice(events, func(i, j int) bool { return events[i].second < events[j].second })
		var prefix, session int64
		for j, event := range events {
			prefix += event.prefix
			session += event.session
			if j == len(events)-1 || event.second != events[j+1].second {
				hours[i].PrefixPeakCandidate = max(hours[i].PrefixPeakCandidate, prefix)
				hours[i].SessionPeakCandidate = max(hours[i].SessionPeakCandidate, session)
			}
		}
	}
	return hours, nil
}
