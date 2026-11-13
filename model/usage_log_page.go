package model

import (
	"context"
	"database/sql"
	"encoding/base64"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type usageLogPageSnapshot struct {
	MaxID int64
	Start int64
	End   int64
	Total int64
	Until int64
}

type NumberedUsageLogPage struct {
	Items        []*Log   `json:"items"`
	DetailRefs   []string `json:"detail_refs"`
	Page         int      `json:"page"`
	PageSize     int      `json:"page_size"`
	Total        int64    `json:"total"`
	PageSnapshot string   `json:"page_snapshot"`
}

// GetNumberedUsageLogPage freezes the time and insertion bounds and counts the
// matching rows once. Subsequent pages reuse those bounds and the count.
func GetNumberedUsageLogPage(ctx context.Context, userID int, filter LogFilter, token string, page, size int, maxRange int64, role int) (NumberedUsageLogPage, error) {
	result := NumberedUsageLogPage{Items: []*Log{}, Page: page, PageSize: size}
	if page < 1 || size < 1 || size > 100 || int64(page-1) > int64(^uint(0)>>1)/int64(size) || len(token) > 2048 {
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
	newSnapshot := snapshot.Until <= now
	if newSnapshot {
		snapshot = usageLogPageSnapshot{
			Start: filter.StartTimestamp,
			End:   filter.EndTimestamp,
			Until: now + int64(time.Hour/time.Second),
		}
		if snapshot.End == 0 {
			snapshot.End = now
		}
		if snapshot.Start == 0 {
			snapshot.Start = snapshot.End - 3600
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
		if userID > 0 {
			query = query.Where("logs.user_id = ?", userID)
			order = "logs.id DESC"
		}
		if newSnapshot {
			if err := db.Model(&Log{}).Select("COALESCE(MAX(id), 0)").Scan(&snapshot.MaxID).Error; err != nil {
				return err
			}
		}
		query = query.Where("logs.id <= ?", snapshot.MaxID)
		if newSnapshot && offset == 0 {
			if err := query.Session(&gorm.Session{}).Order(order).Omit("ip", "content").Limit(size + 1).Find(&result.Items).Error; err != nil {
				return err
			}
			if len(result.Items) <= size {
				snapshot.Total = int64(len(result.Items))
				return nil
			}
			result.Items = result.Items[:size]
		}
		if newSnapshot {
			if err := query.Session(&gorm.Session{}).Count(&snapshot.Total).Error; err != nil {
				return err
			}
		}
		if int64(offset) >= snapshot.Total {
			return nil
		}
		if offset == 0 {
			if newSnapshot {
				return nil
			}
			return query.Order(order).Omit("ip", "content").Limit(size).Find(&result.Items).Error
		}

		// Skip rows using only the sort keys, then load summary columns for
		// the selected page by primary key. MySQL 5.7 supports LIMIT in a
		// derived table joined this way.
		pageIDs := query.Select("logs.id", "logs.created_at").Order(order).Offset(offset).Limit(size)
		return db.Model(&Log{}).Joins("JOIN (?) AS page_ids ON page_ids.id = logs.id", pageIDs).
			Order(order).Omit("ip", "content").Find(&result.Items).Error
	}
	db := LOG_DB.WithContext(ctx)
	if newSnapshot {
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
	if newSnapshot {
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
	result.PageSnapshot = token
	result.DetailRefs, err = formatLogPage(ctx, result.Items, userID, offset, role)
	return result, err
}
