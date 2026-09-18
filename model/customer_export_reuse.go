package model

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// CustomerExportReuse describes a verified source revision and delivery store.
// It is internal: callers cannot supply a revision through the public API.
type CustomerExportReuse struct {
	SourceVersion string
	StoreIdentity string
}

// CustomerStatementExportSourceVersion uses the existing transactional billing
// revisions, including cross-month refund evidence. A log ID or timestamp alone
// cannot prove that historical rows were not corrected. Without this tracking
// (notably split main/log databases), completed exports cannot be auto-reused.
func CustomerStatementExportSourceVersion(ctx context.Context, userID int, jobType string, filters CustomerExportFilters) (string, error) {
	if jobType != CustomerExportJobTypeStatementSummary && jobType != CustomerExportJobTypeStatementDetails {
		return "", nil
	}
	if !ShouldTrackBillingStatementRevision() {
		return "", nil
	}
	for _, logType := range filters.LogTypes {
		if logType != LogTypeConsume && logType != LogTypeRefund {
			return "", nil
		}
	}
	var source struct {
		Revisions   []BillingStatementRevision
		Maintenance BillingStatementMaintenance
		Users       []struct {
			Id       int
			Username string
		}
	}
	query := DB.WithContext(ctx)
	if err := query.Select("scope, revision, generation").Where("scope = ?", fmt.Sprintf("ev:%d:0", userID)).Find(&source.Revisions).Error; err != nil {
		return "", err
	}
	if err := query.First(&source.Maintenance, billingStatementMaintenanceRowID).Error; err != nil {
		return "", err
	}
	if source.Maintenance.Enabled {
		return "", nil
	}
	// Revision-write failures fence the affected customer's retention records.
	var partial int64
	if err := query.Model(&BillingStatementRetention{}).Where("user_id = ? AND status = ?", userID, BillingStatementRetentionPartial).Count(&partial).Error; err != nil {
		return "", err
	}
	if partial > 0 {
		return "", nil
	}
	if err := query.Model(&User{}).Select("id, username").Where("id = ?", userID).Find(&source.Users).Error; err != nil {
		return "", err
	}
	// Only the maintenance generation is a dependency, not its audit timestamps.
	source.Maintenance = BillingStatementMaintenance{Generation: source.Maintenance.Generation}
	raw, err := common.Marshal(source)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v1:%x", sha256.Sum256(raw)), nil
}

func findReusableCustomerExport(tx *gorm.DB, userID, targetID int, jobType, filters string, reuse CustomerExportReuse) (*CustomerExportJob, error) {
	if reuse.SourceVersion == "" || reuse.StoreIdentity == "" {
		return nil, nil
	}
	var jobs []CustomerExportJob
	if err := tx.Where("user_id = ? AND target_user_id = ? AND job_type = ? AND filters = ? AND status = ? AND expires_at > ?",
		userID, targetID, jobType, filters, CustomerExportJobStatusSucceeded, common.GetTimestamp()).Order("id DESC").Limit(20).Find(&jobs).Error; err != nil {
		return nil, err
	}
	for i := range jobs {
		artifact := jobs[i].DecodeArtifact()
		if jobs[i].Filters == filters && artifact != nil && artifact.SourceVersion == reuse.SourceVersion && artifact.StoreIdentity == reuse.StoreIdentity && artifact.ExpiresAt > common.GetTimestamp() {
			return &jobs[i], nil
		}
	}
	return nil, nil
}
