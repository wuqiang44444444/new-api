package model

import (
	"errors"

	"gorm.io/gorm"
)

const retiredErrorReportEnabledOption = "error_report_setting.enabled"

var errRetiredErrorReportEnabledOption = errors.New("the daily report enable option has been retired")

// Retire the report switch and stop pending hourly mail, preserving history and the daily cursor.
func migrateDailyErrorReportSchedule() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&Option{Key: retiredErrorReportEnabledOption}).Error; err != nil {
			return err
		}
		hourly := tx.Model(&ErrorReport{}).Select("report_id").Where("window_end - window_start = ?", 3600)
		return tx.Model(&ErrorReportDelivery{}).Where("report_id IN (?) AND status IN ?", hourly,
			[]string{MailDeliveryPending, MailDeliveryRetryWait, MailDeliverySending}).
			Updates(map[string]any{"status": MailDeliveryStopped, "stop_reason": "daily_report_cutover", "lease_until": nil, "lease_owner": ""}).Error
	})
}
