package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// CustomerExportReuse describes a verified source revision and delivery store.
// It is internal: callers cannot supply a revision through the public API.
type CustomerExportReuse struct {
	SourceVersion string
	StoreIdentity string
	// Regeneration requests a particular frozen snapshot, not just its filters.
	RequireExactActiveFilters bool
}

// CustomerStatementExportSourceVersion uses the existing transactional billing
// revisions, including cross-month refund evidence. A log ID or timestamp alone
// cannot prove that historical rows were not corrected. Without this tracking
// (notably split main/log databases), completed exports cannot be auto-reused.
func CustomerStatementExportSourceVersion(ctx context.Context, userID int, jobType string, filters CustomerExportFilters) (string, error) {
	if jobType != CustomerExportJobTypeStatementSummary && jobType != CustomerExportJobTypeStatementDetails {
		return "", nil
	}
	if filters.StatementDraftId != "" {
		return "statement:" + filters.StatementDraftId, nil
	}
	if filters.BillingReportVersion != "" {
		return fmt.Sprintf("daily:%x", sha256.Sum256([]byte(filters.BillingReportVersion))), nil
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
		ParserVersion int
		Revisions     []BillingStatementRevision
		Maintenance   BillingStatementMaintenance
		Users         []struct {
			Id       int
			Username string
		}
	}
	source.ParserVersion = BillingStatementParserVersion
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

func findReusableCustomerExport(tx *gorm.DB, userID, targetID int, jobType string, filters CustomerExportFilters, filtersText string, reuse CustomerExportReuse) (*CustomerExportJob, error) {
	// The normalized filters already contain the exact immutable daily report generations.
	frozenSelfUsage := jobType == CustomerExportJobTypeUsageSummary && userID == targetID &&
		filters.UsageView == "self" && len(filters.UsageReportDays) > 0
	// Upstream exports require an immutable daily report manifest. Its version
	// and all output-affecting scope fields are part of the exact filter match.
	frozenUpstream := filters.BillingReportVersion != "" && filters.Upstream != nil &&
		(jobType == CustomerExportJobTypeUpstreamDetails || jobType == CustomerExportJobTypeUpstreamSummary)
	if reuse.StoreIdentity == "" || (!frozenSelfUsage && !frozenUpstream && reuse.SourceVersion == "") {
		return nil, nil
	}
	var jobs []CustomerExportJob
	if err := tx.Where("user_id = ? AND target_user_id = ? AND job_type = ? AND filters = ? AND status = ? AND expires_at > ?",
		userID, targetID, jobType, filtersText, CustomerExportJobStatusSucceeded, common.GetTimestamp()).Order("id DESC").Limit(20).Find(&jobs).Error; err != nil {
		return nil, err
	}
	for i := range jobs {
		artifact := jobs[i].DecodeArtifact()
		if jobs[i].Filters == filtersText && artifact != nil &&
			(frozenSelfUsage || frozenUpstream || artifact.SourceVersion == reuse.SourceVersion) &&
			artifact.StoreIdentity == reuse.StoreIdentity && artifact.ExpiresAt > common.GetTimestamp() {
			return &jobs[i], nil
		}
	}
	return nil, nil
}

// CompletedStatementExportCandidates returns the actor's matching snapshots.
// The service verifies the referenced files outside a DB transaction.
func CompletedStatementExportCandidates(ctx context.Context, actorID, targetID int, scopes []CustomerExportFilters, reuse CustomerExportReuse) ([]*CustomerExportJob, error) {
	if reuse.SourceVersion == "" || reuse.StoreIdentity == "" {
		return nil, nil
	}
	encoded := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		raw, err := common.Marshal(scope)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, string(raw))
	}
	var jobs []*CustomerExportJob
	now := common.GetTimestamp()
	if err := DB.WithContext(ctx).
		Where("user_id = ? AND target_user_id = ? AND job_type = ? AND status = ? AND expires_at > ? AND filters IN ?",
			actorID, targetID, CustomerExportJobTypeStatementDetails, CustomerExportJobStatusSucceeded, now, encoded).
		Order("id DESC").Limit(20).Find(&jobs).Error; err != nil {
		return nil, err
	}
	return slices.DeleteFunc(jobs, func(job *CustomerExportJob) bool {
		artifact := job.DecodeArtifact()
		return artifact == nil || len(artifact.Files) == 0 || artifact.ExpiresAt <= now ||
			artifact.SourceVersion != reuse.SourceVersion || artifact.StoreIdentity != reuse.StoreIdentity
	}), nil
}
