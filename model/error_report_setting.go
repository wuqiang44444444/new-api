package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Closed periods contain only hours completed before disabling. End=0 is the
// current enabled period. Persisting periods preserves outage backlog across
// any number of explicit disable/re-enable operations without generating gaps.
type errorReportPeriod struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// UpdateErrorReportOption commits the setting and its effective hour together.
// The native option entry only dispatches these two report-specific keys here.
func UpdateErrorReportOption(key, value string, at time.Time) error {
	if key == "error_report_setting.recipients" {
		_, err := operation_setting.ValidateErrorReportRecipients(value)
		if err != nil {
			return err
		}
	} else if key == "error_report_setting.enabled" {
		if value != "true" && value != "false" {
			return errors.New("invalid report enabled value")
		}
	} else {
		return errors.New("invalid report option")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal}).Error; err != nil {
			return err
		}
		var schedule ErrorReportSchedule
		if err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error; err != nil {
			return err
		}
		enabled := schedule.Enabled
		var option Option
		if err := tx.Where(&Option{Key: "error_report_setting.recipients"}).Find(&option).Error; err != nil {
			return err
		}
		recipients := option.Value
		if key == "error_report_setting.enabled" {
			enabled = value == "true"
		} else {
			recipients = value
		}
		if enabled && len(operation_setting.ParseErrorReportRecipients(recipients)) == 0 {
			return errors.New("启用每小时报告必须配置合法收件人")
		}
		var periods []errorReportPeriod
		if schedule.Periods != "" {
			if err := common.UnmarshalJsonStr(schedule.Periods, &periods); err != nil {
				return err
			}
		}
		hour := at.In(time.FixedZone("Asia/Shanghai", 8*3600)).Truncate(time.Hour).Unix()
		if enabled != schedule.Enabled {
			if enabled {
				if len(periods) > 0 && periods[len(periods)-1].End == hour {
					periods[len(periods)-1].End = 0
				} else {
					periods = append(periods, errorReportPeriod{Start: hour})
				}
				if schedule.BaseWindowStart == 0 {
					schedule.BaseWindowStart = hour
					schedule.NextWindowStart = hour
				}
			} else if len(periods) > 0 {
				periods[len(periods)-1].End = hour
			}
		}
		data, err := common.Marshal(periods)
		if err != nil {
			return err
		}
		schedule.Enabled = enabled
		schedule.Periods = string(data)
		schedule.UpdatedAt = at.Unix()
		if err := tx.Save(&schedule).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&Option{Key: key, Value: value}).Error
	})
	if err != nil {
		return err
	}
	return updateOptionMap(key, value)
}

// Read the main database at execution boundaries: another node's stale option
// cache must not permit sending after the administrator disabled or removed it.
func GetErrorReportConfiguration(ctx context.Context) (bool, []string, error) {
	var schedule ErrorReportSchedule
	if err := DB.WithContext(ctx).Where("scope = ?", ErrorReportScheduleScopeGlobal).Find(&schedule).Error; err != nil {
		return false, nil, err
	}
	if !schedule.Enabled {
		return false, nil, nil
	}
	var option Option
	if err := DB.WithContext(ctx).Where(&Option{Key: "error_report_setting.recipients"}).Find(&option).Error; err != nil {
		return false, nil, err
	}
	recipients, err := operation_setting.ValidateErrorReportRecipients(option.Value)
	return true, recipients, err
}

// NextErrorReportWindow skips explicitly disabled hours while retaining earlier
// completed enabled hours. The cursor still advances only after publication.
func NextErrorReportWindow(ctx context.Context) (int64, error) {
	var next int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var schedule ErrorReportSchedule
		if err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).Find(&schedule).Error; err != nil {
			return err
		}
		if !schedule.Enabled {
			return nil
		}
		var periods []errorReportPeriod
		if err := common.UnmarshalJsonStr(schedule.Periods, &periods); err != nil {
			return err
		}
		for len(periods) > 0 {
			p := periods[0]
			start := max(schedule.NextWindowStart, p.Start)
			if p.End != 0 && start >= p.End {
				periods = periods[1:]
				continue
			}
			next = start
			break
		}
		data, err := common.Marshal(periods)
		if err != nil {
			return err
		}
		return tx.Model(&schedule).Updates(map[string]any{"next_window_start": next, "periods": string(data)}).Error
	})
	return next, err
}
