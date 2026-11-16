package model

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const MailErrorLimit = 5

type MailErrorSample struct {
	ID, UserID, Status                                               int
	CreatedAt                                                        int64
	Username, RequestID, Method, Route, ModelName, Reason, EventType string
}

type MailErrorScope struct {
	Start, End                int64
	UserIDs                   []int // nil is unscoped; an empty slice permits no account.
	RequestIDs                []string
	Reason                    string
	ChannelID                 int
	CustomerOnly              bool
	FailuresOnly              bool
	ExcludePassedChannelTests bool
}

func ReadMailErrorSamples(ctx context.Context, scope MailErrorScope) ([]MailErrorSample, error) {
	if LOG_DB == nil {
		return nil, errors.New("mail error log unavailable")
	}
	clickHouse := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	db := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).
		Select("id, user_id, status, created_at, username, request_id, method, route, model_name, reason, event_type").
		Where("created_at >= ? AND created_at < ?", scope.Start, scope.End)
	if clickHouse {
		// ClickHouse does not assign an event ID. Use the existing log
		// ordering with deterministic tie breakers for the projected fields.
		db = db.Order("created_at DESC, request_id DESC, user_id DESC, event_type DESC, status DESC, reason DESC, method DESC, route DESC, model_name DESC, username DESC, id DESC")
	} else {
		db = db.Order("created_at DESC, id DESC")
	}
	if scope.Reason != "" {
		db = db.Where("event_type = ? AND reason = ?", clienterrlog.EventAPIError, scope.Reason)
	}
	if scope.ChannelID > 0 {
		db = db.Where("channel_id = ?", scope.ChannelID)
	}
	if scope.RequestIDs != nil {
		db = db.Where("request_id IN ?", scope.RequestIDs)
	}
	if scope.FailuresOnly {
		db = db.Where("((event_type = ? AND status >= ?) OR event_type = ? OR (event_type = ? AND reason NOT IN ?))",
			clienterrlog.EventAPIError, 400, clienterrlog.EventTaskFailure, clienterrlog.EventStreamError,
			[]string{"client_disconnected", "upstream_response_cancelled", "stream_soft_errors", "stream_stopped_with_error"})
	}
	if scope.ExcludePassedChannelTests {
		db = withoutPassedChannelTests(db)
	}
	if scope.UserIDs != nil {
		var newest []MailErrorSample
		for start := 0; start < len(scope.UserIDs); start += 200 {
			var rows []MailErrorSample
			if err := db.Session(&gorm.Session{}).Where("user_id IN ?", scope.UserIDs[start:min(start+200, len(scope.UserIDs))]).Limit(MailErrorLimit).Scan(&rows).Error; err != nil {
				return nil, err
			}
			newest = append(newest, rows...)
			slices.SortFunc(newest, func(a, b MailErrorSample) int {
				if order := cmp.Compare(b.CreatedAt, a.CreatedAt); order != 0 {
					return order
				}
				if clickHouse {
					return cmp.Or(cmp.Compare(b.RequestID, a.RequestID), cmp.Compare(b.UserID, a.UserID), cmp.Compare(b.EventType, a.EventType),
						cmp.Compare(b.Status, a.Status), cmp.Compare(b.Reason, a.Reason), cmp.Compare(b.Method, a.Method),
						cmp.Compare(b.Route, a.Route), cmp.Compare(b.ModelName, a.ModelName), cmp.Compare(b.Username, a.Username), cmp.Compare(b.ID, a.ID))
				}
				return cmp.Compare(b.ID, a.ID)
			})
			newest = newest[:min(MailErrorLimit, len(newest))]
		}
		return newest, nil
	}
	if !scope.CustomerOnly {
		var samples []MailErrorSample
		err := db.Limit(MailErrorLimit).Scan(&samples).Error
		return samples, err
	}
	// The error store may be separate from the main DB. Check account roles
	// with bounded pages before selecting the five customer events.
	var samples []MailErrorSample
	var cursor MailErrorSample
	const pageSize = 64
	offset := 0
	for {
		q := db.Session(&gorm.Session{}).Where("user_id > 0")
		if clickHouse {
			q = q.Offset(offset)
		} else if offset > 0 {
			q = q.Where("(created_at < ? OR (created_at = ? AND id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		}
		var rows []MailErrorSample
		if err := q.Limit(pageSize).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return samples, nil
		}
		ids := make([]int, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UserID)
		}
		accounts, err := ReadMailAccounts(DB.WithContext(ctx).Unscoped(), ids)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if account, ok := accounts[row.UserID]; ok && account.Role == common.RoleCommonUser {
				samples = append(samples, row)
				if len(samples) == MailErrorLimit {
					return samples, nil
				}
			}
		}
		if len(rows) < pageSize {
			return samples, nil
		}
		cursor = rows[len(rows)-1]
		offset += len(rows)
	}
}

type MailAccount struct {
	ID, Role              int
	Username, DisplayName string
}

func ReadMailAccounts(db *gorm.DB, ids []int) (map[int]MailAccount, error) {
	accounts := make(map[int]MailAccount, len(ids))
	if len(ids) == 0 {
		return accounts, nil
	}
	var rows []MailAccount
	if err := db.Model(&User{}).Select("id, role, username, display_name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		accounts[row.ID] = row
	}
	return accounts, nil
}

// ChannelMailObservation is a bounded projection of a counted incident sample.
type ChannelMailObservation struct {
	At                int64
	Status            int
	RequestID, Reason string
}
