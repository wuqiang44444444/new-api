package model

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type usageLogPageSnapshot struct {
	MaxID        int64
	Start        int64
	End          int64
	Total        int64
	TotalPending bool
	Until        int64
}

type NumberedUsageLogPage struct {
	Items        []*Log   `json:"items"`
	DetailRefs   []string `json:"detail_refs"`
	Page         int      `json:"page"`
	PageSize     int      `json:"page_size"`
	Total        int64    `json:"total"`
	PageSnapshot string   `json:"page_snapshot"`
	HasMore      bool     `json:"has_more"`
	TotalPending bool     `json:"total_pending,omitempty"`
	RangeStart   int64    `json:"start_timestamp,omitempty"`
	RangeEnd     int64    `json:"end_timestamp,omitempty"`
}

type UsageLogTotalMode string

const (
	UsageLogTotalDeferred UsageLogTotalMode = "defer"
	UsageLogTotalOnly     UsageLogTotalMode = "count"
	UsageLogLastPage      UsageLogTotalMode = "last"
)

var ErrUsageLogSnapshotExpired = errors.New("usage log snapshot expired")

func GetNumberedUsageLogPage(ctx context.Context, userID int, filter LogFilter, token string, page, size int, maxRange int64, role int) (NumberedUsageLogPage, error) {
	return GetUsageLogPage(ctx, userID, filter, token, page, size, maxRange, role, "")
}

func GetUsageLogPage(ctx context.Context, userID int, filter LogFilter, token string, page, size int, maxRange int64, role int, mode UsageLogTotalMode) (NumberedUsageLogPage, error) {
	result := NumberedUsageLogPage{Items: []*Log{}, Page: page, PageSize: size}
	if page < 1 || size < 1 || size > 100 || int64(page-1) > int64(^uint(0)>>1)/int64(size) ||
		len(token) > 2048 || ((mode == UsageLogTotalOnly || mode == UsageLogLastPage) && token == "start") {
		return result, common.NewMessage("Invalid log pagination")
	}
	offset := (page - 1) * size
	now := time.Now().Unix()
	rawScope, err := common.Marshal(struct {
		UserID int
		LogFilter
	}{userID, filter})
	if err != nil {
		return result, err
	}
	scope := "usage-log-page:" + base64.RawURLEncoding.EncodeToString(common.Sha256Raw(rawScope))
	if mode != "" {
		scope = "deferred:" + scope
	}
	var snapshot usageLogPageSnapshot
	if token != "start" {
		if !strings.HasPrefix(token, "v2.") {
			return result, common.NewMessage("Invalid log pagination")
		}
		raw, err := common.DecryptShortLivedSecretForScope(scope, token)
		if err != nil || common.UnmarshalJsonStr(raw, &snapshot) != nil {
			return result, common.NewMessage("Invalid log pagination")
		}
	}
	if mode != "" && token != "start" && snapshot.Until <= now {
		return result, ErrUsageLogSnapshotExpired
	}
	newSnapshot := snapshot.Until <= now
	updateToken := newSnapshot
	if newSnapshot {
		start, end := usageLogReadRange(filter, now, maxRange, mode != "")
		snapshot = usageLogPageSnapshot{
			Start: start,
			End:   end,
			Until: now + int64(time.Hour/time.Second),
		}
	}
	filter.StartTimestamp, filter.EndTimestamp = snapshot.Start, snapshot.End
	if filter.StartTimestamp > filter.EndTimestamp || (maxRange > 0 && filter.StartTimestamp < min(now, filter.EndTimestamp)-maxRange) {
		return result, common.NewMessage("Invalid log pagination")
	}

	readPage := func(db *gorm.DB) error {
		query, err := filter.apply(db.Model(&Log{}))
		if err != nil {
			return err
		}
		order := "logs.created_at DESC, logs.id DESC"
		reverseOrder := "logs.created_at ASC, logs.id ASC"
		if userID > 0 {
			query = query.Where("logs.user_id = ?", userID)
			order = "logs.id DESC"
			reverseOrder = "logs.id ASC"
		}
		if newSnapshot {
			if err := db.Model(&Log{}).Select("COALESCE(MAX(id), 0)").Scan(&snapshot.MaxID).Error; err != nil {
				return err
			}
		}
		query = query.Where("logs.id <= ?", snapshot.MaxID)
		if mode == UsageLogLastPage {
			if err := query.Session(&gorm.Session{}).Count(&snapshot.Total).Error; err != nil {
				return err
			}
			snapshot.TotalPending, updateToken = false, true
			if snapshot.Total == 0 {
				result.Page, offset = 1, 0
				return nil
			}
			lastIndex := (snapshot.Total - 1) / int64(size)
			if lastIndex > int64(^uint(0)>>1)/int64(size) {
				return common.NewMessage("Invalid log pagination")
			}
			result.Page, offset = int(lastIndex)+1, int(lastIndex)*size
			lastIDs := query.Session(&gorm.Session{}).Select("logs.id", "logs.created_at").Order(reverseOrder).Limit(int(snapshot.Total - int64(offset)))
			return db.Model(&Log{}).Joins("JOIN (?) AS page_ids ON page_ids.id = logs.id", lastIDs).
				Order(order).Omit("ip", "content").Find(&result.Items).Error
		}
		if mode == UsageLogTotalOnly {
			if snapshot.TotalPending {
				if err := query.Count(&snapshot.Total).Error; err != nil {
					return err
				}
				snapshot.TotalPending = false
				updateToken = true
			}
			return nil
		}

		if !newSnapshot && !snapshot.TotalPending && int64(offset) >= snapshot.Total {
			return nil
		}
		limit := size
		probe := (newSnapshot && offset == 0) || mode == UsageLogTotalDeferred
		if probe {
			limit++
		}
		if (newSnapshot && !probe) || (snapshot.TotalPending && mode == "") {
			if err := query.Session(&gorm.Session{}).Count(&snapshot.Total).Error; err != nil {
				return err
			}
			snapshot.TotalPending = false
			updateToken = true
			if int64(offset) >= snapshot.Total {
				return nil
			}
		}

		if offset == 0 {
			if err := query.Session(&gorm.Session{}).Order(order).Omit("ip", "content").Limit(limit).Find(&result.Items).Error; err != nil {
				return err
			}
		} else {
			// Skip rows using only the sort keys, then load the selected summaries by primary key.
			pageIDs := query.Session(&gorm.Session{}).Select("logs.id", "logs.created_at").Order(order).Offset(offset).Limit(limit)
			if err := db.Model(&Log{}).Joins("JOIN (?) AS page_ids ON page_ids.id = logs.id", pageIDs).
				Order(order).Omit("ip", "content").Find(&result.Items).Error; err != nil {
				return err
			}
		}

		if len(result.Items) > size {
			result.HasMore = true
			result.Items = result.Items[:size]
		}
		if newSnapshot && probe {
			switch {
			case offset == 0 && !result.HasMore:
				snapshot.Total = int64(len(result.Items))
			case mode == UsageLogTotalDeferred:
				snapshot.TotalPending = true
			default:
				return query.Session(&gorm.Session{}).Count(&snapshot.Total).Error
			}
		}
		if !probe {
			result.HasMore = snapshot.Total-int64(offset) > int64(len(result.Items))
		}
		return nil
	}
	db := LOG_DB.WithContext(ctx)
	if newSnapshot || mode == UsageLogLastPage {
		options := sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}
		if db.Dialector.Name() == "sqlite" {
			options.Isolation = sql.LevelDefault
		}
		err = db.Transaction(readPage, &options)
	} else {
		err = readPage(db)
	}
	if err != nil {
		return result, err
	}
	if updateToken {
		raw, err := common.Marshal(snapshot)
		if err != nil {
			return result, err
		}
		token, err = common.EncryptShortLivedSecretForScope(scope, string(raw))
		if err != nil {
			return result, err
		}
	}
	result.Total = snapshot.Total
	result.TotalPending = snapshot.TotalPending
	result.RangeStart, result.RangeEnd = snapshot.Start, snapshot.End
	result.PageSnapshot = token
	result.DetailRefs, err = formatLogPage(ctx, result.Items, userID, offset, role)
	return result, err
}

// Exact request lookups without an explicit time filter use the caller's
// allowed range. Ordinary browsing still starts at the most recent hour.
func usageLogReadRange(filter LogFilter, now, maxRange int64, allowFullRequestLookup bool) (start, end int64) {
	start, end = filter.StartTimestamp, filter.EndTimestamp
	if end == 0 {
		end = now
	}
	if start == 0 {
		switch {
		case allowFullRequestLookup && filter.RequestId != "" && filter.EndTimestamp == 0:
			if maxRange > 0 {
				start = end - maxRange
			}
		default:
			start = end - 3600
		}
	}
	return start, end
}
