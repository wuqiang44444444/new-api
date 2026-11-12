package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DailyReportWindowSeconds int64 = 24 * 60 * 60

func migrateDailyReportWindowIndex() error {
	const hourlyWindowIndex = "idx_error_reports_window_start"
	if DB.Migrator().HasIndex(&ErrorReport{}, hourlyWindowIndex) {
		return DB.Migrator().DropIndex(&ErrorReport{}, hourlyWindowIndex)
	}
	return nil
}

func DailyReportCursor(ctx context.Context, initialDay int64) (int64, error) {
	db := DB.WithContext(ctx)
	var schedule ErrorReportSchedule
	result := db.Where("scope = ?", ErrorReportScheduleScopeGlobal).Find(&schedule)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		initial := ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal, DailyWindowStart: initialDay}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return 0, err
		}
	} else {
		if schedule.DailyWindowStart != 0 {
			return schedule.DailyWindowStart, nil
		}
		claim := db.Model(&ErrorReportSchedule{}).
			Where("scope = ? AND (daily_window_start = 0 OR daily_window_start IS NULL)", ErrorReportScheduleScopeGlobal).
			UpdateColumn("daily_window_start", initialDay)
		if claim.Error != nil || claim.RowsAffected == 1 {
			return initialDay, claim.Error
		}
	}
	schedule = ErrorReportSchedule{}
	err := db.Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error
	return schedule.DailyWindowStart, err
}

func AdvanceDailyReportCursor(ctx context.Context, start int64) error {
	return DB.WithContext(ctx).Model(&ErrorReportSchedule{}).
		Where("scope = ? AND daily_window_start = ?", ErrorReportScheduleScopeGlobal, start).
		Update("daily_window_start", start+DailyReportWindowSeconds).Error
}

func LoadDailyReport(ctx context.Context, start int64, id string) (*ErrorReport, error) {
	var existing ErrorReport
	found := DB.WithContext(ctx).Where("report_id = ?", id).Find(&existing)
	if found.Error != nil {
		return nil, found.Error
	}
	if found.RowsAffected != 0 {
		return &existing, nil
	}
	report := ErrorReport{ReportID: id, WindowStart: start, WindowEnd: start + DailyReportWindowSeconds,
		Timezone: "Asia/Shanghai", Status: ErrorReportStatusBuilding, AggregatedThrough: start}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&report).Error; err != nil {
		return nil, err
	}
	if report.ID == 0 {
		if err := DB.WithContext(ctx).Where("report_id = ?", id).First(&report).Error; err != nil {
			return nil, err
		}
	}
	return &report, nil
}

func SaveDailyReportProgress(ctx context.Context, report *ErrorReport, next int64, summary string, total int64) error {
	result := DB.WithContext(ctx).Model(&ErrorReport{}).
		Where("report_id = ? AND status = ? AND aggregated_through = ?", report.ReportID, ErrorReportStatusBuilding, report.AggregatedThrough).
		Updates(map[string]any{"aggregated_through": next, "summary": summary, "total_events": total})
	if result.Error == nil && result.RowsAffected != 1 {
		return errors.New("daily report progress changed")
	}
	return result.Error
}

type DailyReportCount struct {
	Key   string `gorm:"column:label"`
	Count int64
}

type DailyReportChannelCount struct {
	ChannelID int
	Count     int64
}

type DailyReportSlice struct {
	Total    int64
	Errors   []DailyReportCount
	Channels []DailyReportChannelCount
}

func ReadDailyReportSlice(ctx context.Context, start, end int64) (DailyReportSlice, error) {
	var slice DailyReportSlice
	if LOG_DB == nil {
		return slice, errors.New("error log database unavailable")
	}
	query := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).Where("created_at >= ? AND created_at < ?", start, end)
	// One grouped scan handles the usual case. Only an unusually diverse
	// slice needs a second count to keep its total independent of rankings.
	const maxGroups = 128
	var groups []struct {
		EventType string
		ChannelID int
		Count     int64
	}
	if err := query.Session(&gorm.Session{}).Select("event_type, channel_id, COUNT(*) AS count").
		Group("event_type, channel_id").Order("count DESC, event_type ASC, channel_id ASC").
		Limit(maxGroups + 1).Scan(&groups).Error; err != nil {
		return slice, err
	}
	errorsByType := make(map[string]int64)
	channels := make(map[int]int64)
	for _, group := range groups[:min(len(groups), maxGroups)] {
		slice.Total += group.Count
		errorsByType[group.EventType] += group.Count
		if group.ChannelID > 0 {
			channels[group.ChannelID] += group.Count
		}
	}
	if len(groups) > maxGroups {
		if err := query.Session(&gorm.Session{}).Count(&slice.Total).Error; err != nil {
			return slice, err
		}
	}
	for label, count := range errorsByType {
		slice.Errors = append(slice.Errors, DailyReportCount{Key: label, Count: count})
	}
	for id, count := range channels {
		slice.Channels = append(slice.Channels, DailyReportChannelCount{ChannelID: id, Count: count})
	}
	return slice, nil
}

func ReadDailyOperationsCounts(ctx context.Context, start, end int64) ([]DailyReportCount, []DailyReportCount, error) {
	var tasks, resources []DailyReportCount
	if err := DB.WithContext(ctx).Model(&SystemTask{}).
		Where("updated_at >= ? AND updated_at < ? AND status = ? AND type <> ?", start, end, SystemTaskStatusFailed, SystemTaskTypeErrorReport).
		Select("type AS label, COUNT(*) AS count").Group("type").Order("count DESC, type ASC").Limit(24).Scan(&tasks).Error; err != nil {
		return nil, nil, err
	}
	err := DB.WithContext(ctx).Model(&ResourceAlertEpisode{}).Where("opened_at >= ? AND opened_at < ?", start, end).
		Select("resource AS label, COUNT(*) AS count").Group("resource").Order("resource ASC").Scan(&resources).Error
	return tasks, resources, err
}

// Limit each statement batch by both the parent task and a short database deadline.
const DailyReportReadBudget = 2 * time.Second
