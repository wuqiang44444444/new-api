package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// One-time conversion of the pre-release schedule. Old code did not record
// disable times, so those intervals cannot be reconstructed. Preserve the old
// cursor if currently enabled and all frozen deliveries; when disabled, restart
// generation only on the next explicit enable. Runtime has no dual-read path.
func migrateErrorReportPeriods() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var option Option
		if err := tx.Where(&Option{Key: "error_report_setting.enabled"}).Find(&option).Error; err != nil {
			return err
		}
		var schedule ErrorReportSchedule
		if err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).Find(&schedule).Error; err != nil {
			return err
		}
		if schedule.ID == 0 && option.Value != "true" {
			return nil
		}
		if schedule.Periods != "" {
			return nil
		}
		schedule.Scope = ErrorReportScheduleScopeGlobal
		schedule.Enabled = option.Value == "true"
		periods := []errorReportPeriod{}
		if schedule.Enabled {
			start := schedule.NextWindowStart
			if start == 0 {
				start = time.Now().Truncate(time.Hour).Unix()
			}
			if schedule.BaseWindowStart == 0 {
				schedule.BaseWindowStart = start
			}
			schedule.NextWindowStart = start
			periods = append(periods, errorReportPeriod{Start: start})
		}
		data, err := common.Marshal(periods)
		if err != nil {
			return err
		}
		schedule.Periods = string(data)
		return tx.Save(&schedule).Error
	})
}
