package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type dailyReportExportReference struct {
	Generation string `gorm:"size:32;primaryKey"`
	JobID      string `gorm:"size:64;primaryKey;index"`
}

// Admission and retirement lock the same report rows. The export slot and
// references commit together, so cleanup cannot race an accepted export.
func pinDailyReportExport(tx *gorm.DB, jobID string, filters CustomerExportFilters) error {
	usage := append([]UsageReportDayReference(nil), filters.UsageReportDays...)
	sort.Slice(usage, func(i, j int) bool { return usage[i].Generation < usage[j].Generation })
	var refs []dailyReportExportReference
	for _, ref := range usage {
		var marker usageReportDayRow
		err := lockForUpdate(tx).Where("generation = ? AND user_id = -1 AND payload <> ? AND COALESCE(unavailable_at, 0) = 0", ref.Generation, "").Take(&marker).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.NewMessage("Usage report version is unavailable")
		}
		if err != nil {
			return err
		}
		refs = append(refs, dailyReportExportReference{Generation: ref.Generation, JobID: jobID})
	}
	if filters.BillingReportVersion != "" && filters.BillingReportVersion != "-" {
		ids := strings.Split(filters.BillingReportVersion, ",")
		sort.Strings(ids)
		for _, id := range ids {
			var generation billingReportGeneration
			err := lockForUpdate(tx).Where("id = ? AND published = ?", id, true).Take(&generation).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewMessage("Billing report version is unavailable")
			}
			if err != nil {
				return err
			}
			refs = append(refs, dailyReportExportReference{Generation: id, JobID: jobID})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return tx.Create(&refs).Error
}

func activeDailyReportExports(db *gorm.DB, generation string) *gorm.DB {
	return db.Table("daily_report_export_references AS ref").
		Joins("JOIN customer_export_jobs AS job ON job.job_id = ref.job_id").
		Where("ref.generation = ? AND job.status IN ?", generation,
			[]CustomerExportJobStatus{CustomerExportJobStatusQueued, CustomerExportJobStatusRunning, CustomerExportJobStatusCancelWait})
}

// Job records have their own retention. Release orphan references in bounded
// writes even when the corresponding published generation stays current.
func PruneDailyReportExportReferences(db *gorm.DB) error {
	var refs []dailyReportExportReference
	err := db.Where("NOT EXISTS (SELECT 1 FROM customer_export_jobs job WHERE job.job_id = daily_report_export_references.job_id)").Limit(100).Find(&refs).Error
	if err != nil || len(refs) == 0 {
		return err
	}
	return db.Delete(&refs).Error
}
