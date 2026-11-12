package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// A request can log an error and a successful retry in the same second.
const clickHouseLogPageOrder = "logs.created_at desc, logs.request_id desc, logs.type desc, logs.channel_id desc, logs.user_id desc, logs.id desc"

type logPageCursor struct {
	ID    int    `json:"i"`
	Time  int64  `json:"t"`
	Start int64  `json:"s"`
	End   int64  `json:"e"`
	Scope string `json:"f"`
}

type LogCursorPage struct {
	Items      []*Log   `json:"items"`
	DetailRefs []string `json:"detail_refs"`
	NextCursor string   `json:"next_cursor"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	Total      int      `json:"total"`
}

func GetLogCursorPage(ctx context.Context, userID int, filter LogFilter, cursor string, page, size int, maxRange int64, role int) (LogCursorPage, error) {
	result := LogCursorPage{Items: []*Log{}, Page: page, PageSize: size}
	if page < 1 || page > 1_000_000 || size < 1 || size > 100 || len(cursor) > 2048 {
		return result, common.NewMessage("Invalid log pagination")
	}

	var after logPageCursor
	first := cursor == "start"
	if !first {
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || common.Unmarshal(data, &after) != nil || after.Time <= 0 {
			return result, common.NewMessage("Invalid log pagination")
		}
	}
	if filter.EndTimestamp == 0 {
		filter.EndTimestamp = after.End
		if first {
			filter.EndTimestamp = time.Now().Unix()
		}
	}
	if filter.StartTimestamp == 0 {
		filter.StartTimestamp = after.Start
		if first {
			filter.StartTimestamp = filter.EndTimestamp - 3600
		}
	}
	if filter.StartTimestamp > filter.EndTimestamp || (maxRange > 0 && filter.StartTimestamp < min(time.Now().Unix(), filter.EndTimestamp)-maxRange) {
		return result, common.NewMessage("Invalid log pagination")
	}
	scopeBytes, err := common.Marshal(struct {
		User int
		Size int
		LogFilter
	}{userID, size, filter})
	if err != nil {
		return result, err
	}
	scope := base64.RawURLEncoding.EncodeToString(common.Sha256Raw(scopeBytes))
	if !first && after.Scope != scope {
		return result, common.NewMessage("Invalid log pagination")
	}

	query, err := filter.apply(LOG_DB.WithContext(ctx).Model(&Log{}))
	if err != nil {
		return result, err
	}
	if userID > 0 {
		query = query.Where("logs.user_id = ?", userID)
	}
	switch {
	case userID > 0:
		query = query.Order("logs.id DESC")
		if !first {
			query = query.Where("logs.id < ?", after.ID)
		}
	default:
		query = query.Order("logs.created_at DESC, logs.id DESC")
		if !first {
			query = query.Where("logs.created_at <= ? AND (logs.created_at < ? OR logs.id < ?)", after.Time, after.Time, after.ID)
		}
	}
	if err := query.Omit("ip", "content").Limit(size + 1).Find(&result.Items).Error; err != nil {
		return result, err
	}
	if len(result.Items) > size {
		last := result.Items[size-1]
		data, err := common.Marshal(logPageCursor{
			ID: last.Id, Time: last.CreatedAt,
			Start: filter.StartTimestamp, End: filter.EndTimestamp, Scope: scope,
		})
		if err != nil {
			return result, err
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(data)
		result.Items = result.Items[:size]
	}
	result.DetailRefs = make([]string, 0, len(result.Items))
	expires := time.Now().Add(time.Hour).Unix()
	for _, row := range result.Items {
		raw, err := common.Marshal(logPageCursor{ID: row.Id, Time: row.CreatedAt, End: expires})
		if err != nil {
			return result, err
		}
		ref, err := common.EncryptShortLivedSecretForScope(logDetailScope(userID), string(raw))
		if err != nil {
			return result, err
		}
		result.DetailRefs = append(result.DetailRefs, ref)
	}
	offset := (page - 1) * size
	result.Total = offset + len(result.Items)
	if result.NextCursor != "" {
		result.Total++
	}
	if userID > 0 {
		formatUserLogs(result.Items, offset)
	} else {
		if role < common.RoleRootUser {
			FormatAdminLogs(result.Items)
		} else {
			FormatRootLogs(result.Items)
		}
		ids := make([]int, 0, len(result.Items))
		for _, log := range result.Items {
			if log.ChannelId > 0 {
				ids = append(ids, log.ChannelId)
			}
		}
		if len(ids) > 0 {
			var rows []Channel
			if err := DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error; err != nil {
				return result, err
			}
			names := make(map[int]string, len(rows))
			for _, row := range rows {
				names[row.Id] = row.Name
			}
			for _, log := range result.Items {
				log.ChannelName = names[log.ChannelId]
			}
		}
	}
	for _, row := range result.Items {
		row.Other = logCursorSummaryOther(row.Other)
	}
	return result, nil
}

// List cells use these fields for the event's billed amount, tokens and timing.
// Pricing explanations, raw response diagnostics and privileged metadata are
// available only on the separately authorized detail request.
func logCursorSummaryOther(raw string) string {
	var source map[string]json.RawMessage
	if common.UnmarshalJsonStr(raw, &source) != nil {
		return ""
	}
	summary := make(map[string]json.RawMessage)
	for _, key := range []string{
		"billing_source", "subscription_consumed", "task_id", "task_billing_event",
		"pre_consumed_quota", "adjustment_before_quota", "actual_quota", "net_quota",
		"group", "group_ratio", "user_group_ratio", "cache_tokens", "cache_creation_tokens",
		"cache_creation_tokens_5m", "cache_creation_tokens_1h", "cache_write_unavailable",
		"frt", "is_task", "task_sync", "is_model_mapped", "upstream_model_name", "response_model",
	} {
		if value, ok := source[key]; ok {
			summary[key] = value
		}
	}
	var stream map[string]json.RawMessage
	if common.Unmarshal(source["stream_status"], &stream) == nil && stream != nil {
		preview := make(map[string]json.RawMessage)
		for _, key := range []string{"status", "end_reason", "error_count", "response_status"} {
			if value, ok := stream[key]; ok {
				preview[key] = value
			}
		}
		if encoded, err := common.Marshal(preview); err == nil {
			summary["stream_status"] = encoded
		}
	}
	var admin map[string]json.RawMessage
	if common.Unmarshal(source[logOtherAdminInfoKey], &admin) == nil {
		preview := make(map[string]json.RawMessage)
		for _, key := range []string{"use_channel", "channel_affinity", "is_multi_key", "multi_key_index"} {
			if value, ok := admin[key]; ok {
				preview[key] = value
			}
		}
		if len(preview) > 0 {
			if encoded, err := common.Marshal(preview); err == nil {
				summary[logOtherAdminInfoKey] = encoded
			}
		}
	}
	if len(summary) == 0 {
		return ""
	}
	data, err := common.Marshal(summary)
	if err != nil {
		return ""
	}
	return string(data)
}

type LogRangeStatistics struct {
	Stat
	Total int64 `json:"total"`
}

func GetLogRangeStatistics(ctx context.Context, userID int, filter LogFilter, maxRange int64) (LogRangeStatistics, error) {
	var result LogRangeStatistics
	now := time.Now().Unix()
	if filter.EndTimestamp == 0 {
		filter.EndTimestamp = now
	}
	if filter.StartTimestamp == 0 {
		filter.StartTimestamp = filter.EndTimestamp - 3600
	}
	if filter.StartTimestamp > filter.EndTimestamp || (maxRange > 0 && filter.StartTimestamp < min(now, filter.EndTimestamp)-maxRange) {
		return result, common.NewMessage("Invalid time range")
	}
	query, err := filter.apply(LOG_DB.WithContext(ctx).Model(&Log{}))
	if err != nil {
		return result, err
	}
	if userID > 0 {
		query = query.Where("logs.user_id = ?", userID)
	}
	if err := query.Session(&gorm.Session{}).Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS quota", LogTypeConsume).Scan(&result).Error; err != nil {
		return result, err
	}
	if filter.Type != LogTypeUnknown && filter.Type != LogTypeConsume {
		return result, nil
	}
	var rate Stat
	err = query.Where("logs.type = ? AND logs.created_at >= ?", LogTypeConsume, now-60).
		Select("COUNT(*) AS rpm, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS tpm").
		Scan(&rate).Error
	result.Rpm, result.Tpm = rate.Rpm, rate.Tpm
	return result, err
}

func logDetailScope(userID int) string {
	return fmt.Sprintf("usage-log-detail:%d", userID)
}

func GetLogCursorDetail(ctx context.Context, userID int, reference string) (*Log, error) {
	if len(reference) > 2048 || !strings.HasPrefix(reference, "v2.") {
		return nil, common.NewMessage("Invalid log reference")
	}
	raw, err := common.DecryptShortLivedSecretForScope(logDetailScope(userID), reference)
	var pointer logPageCursor
	if err != nil || common.UnmarshalJsonStr(raw, &pointer) != nil || pointer.End < time.Now().Unix() {
		return nil, common.NewMessage("Invalid log reference")
	}
	query := LOG_DB.WithContext(ctx).Model(&Log{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var rows []*Log
	query = query.Where("id = ? AND created_at = ?", pointer.ID, pointer.Time)
	if err := query.Limit(2).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, common.NewMessage("Log record is unavailable")
	}
	if userID > 0 {
		formatUserLogs(rows, 0)
	}
	return rows[0], nil
}
