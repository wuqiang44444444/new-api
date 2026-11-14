package model

import (
	"context"

	"github.com/QuantumNous/new-api/clienterrlog"
	"gorm.io/gorm"
)

const (
	DailyErrorAPI5xx             = "api_5xx"
	DailyErrorAPI429             = "api_429"
	DailyErrorAPI4xx             = "api_4xx"
	DailyErrorStreamWarning      = "stream_warning"
	DailyErrorStreamDisconnected = "stream_disconnected"
	DailyErrorOther              = "other"
)

// DailyReportErrorKey contains only structured fields that can be used to
// filter the administrator's existing error log. Channel probes have a
// separate upstream HTTP status in their protected detail.
type DailyReportErrorKey struct {
	EventType string `json:"event_type"`
	Status    int    `json:"status"`
	ChannelID int    `json:"channel_id"`
	ModelName string `json:"model_name"`
	Reason    string `json:"reason"`
}

func (key DailyReportErrorKey) Signal() string {
	switch key.EventType {
	case clienterrlog.EventAPIError:
		switch key.Reason {
		case clienterrlog.WalletQuotaInsufficient, clienterrlog.SubscriptionQuotaInsufficient, clienterrlog.APIKeyQuotaInsufficient:
			return key.Reason
		}
		switch {
		case key.Status >= 500 && key.Status < 600:
			return DailyErrorAPI5xx
		case key.Status == 429:
			return DailyErrorAPI429
		case key.Status >= 400 && key.Status < 500:
			return DailyErrorAPI4xx
		}
	case clienterrlog.EventStreamError:
		switch key.Reason {
		case "client_disconnected", "upstream_response_cancelled":
			return DailyErrorStreamDisconnected
		case "stream_soft_errors", "stream_stopped_with_error":
			return DailyErrorStreamWarning
		}
		return key.EventType
	case clienterrlog.EventTaskFailure, clienterrlog.EventChannelTest:
		return key.EventType
	}
	return DailyErrorOther
}

type DailyReportErrorGroup struct {
	DailyReportErrorKey
	Count  int64 `json:"count"`
	LastAt int64 `json:"last_at"`
}

func ReadDailyReportChannelIncidents(ctx context.Context, start, end int64) ([]ChannelAlertEpisode, int64, error) {
	query := DB.WithContext(ctx).Model(&ChannelAlertEpisode{}).
		Where("preview = ? AND confirmed_at < ? AND (closed_at = 0 OR closed_at > ?)", false, end*1000, start*1000)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil || total == 0 {
		return nil, total, err
	}
	var incidents []ChannelAlertEpisode
	err := query.Select("channel_id, model, operation, kind, status_code, confirmed_at, closed_at, close_reason, evidence_count").
		Order("CASE WHEN closed_at = 0 THEN 0 ELSE 1 END, confirmed_at DESC, id DESC").Limit(6).Find(&incidents).Error
	return incidents, total, err
}

func ReadDailyReportChannelNames(ctx context.Context, ids []int) (map[int]string, error) {
	names := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var channels []Channel
	if err := DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		names[channel.Id] = channel.Name
	}
	return names, nil
}

// ReadDailyQuotaRejectionAccounts counts distinct account owners in the retained
// window. An owner may appear once in each independently observed category.
func ReadDailyQuotaRejectionAccounts(ctx context.Context, start, end int64) (map[string]int64, error) {
	var rows []DailyReportCount
	err := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).
		Where("created_at >= ? AND created_at < ? AND event_type = ? AND user_id > 0 AND reason IN ?", start, end, clienterrlog.EventAPIError,
			[]string{clienterrlog.WalletQuotaInsufficient, clienterrlog.SubscriptionQuotaInsufficient, clienterrlog.APIKeyQuotaInsufficient}).
		Select("reason AS label, COUNT(DISTINCT user_id) AS count").Group("reason").Scan(&rows).Error
	accounts := make(map[string]int64, len(rows))
	for _, row := range rows {
		accounts[row.Key] = row.Count
	}
	return accounts, err
}
