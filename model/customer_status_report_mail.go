package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

const CustomerStatusReportMail = "customer_status_report"

type CustomerStatusReport struct {
	NewAccounts              int64
	EnabledAccounts          int64
	DisabledAccounts         int64
	NonPositiveWallets       int64
	PositiveLowWallets       int64
	WalletLowThreshold       int
	AccountsWithCalls        int64
	NewAccountsWithCalls     int64
	AccountsWithFailures     int64
	AccountsWithOnlyFailures int64
}

// ReadCustomerStatusReport pairs the published call outcomes with account
// records. Only ordinary customer accounts are counted. Status and balance are
// current at the time of collection, not historical reconstructions.
func ReadCustomerStatusReport(ctx context.Context, day UsageReportDayReference, end int64) (CustomerStatusReport, error) {
	result := CustomerStatusReport{WalletLowThreshold: common.QuotaRemindThreshold}
	db := DB.WithContext(ctx)
	// Count signups that were subsequently soft-deleted, while excluding deleted
	// rows from the current account status and wallet counts.
	err := db.Unscoped().Model(&User{}).Where("role = ?", common.RoleCommonUser).Select(`
		COUNT(CASE WHEN created_at >= ? AND created_at < ? THEN 1 END) AS new_accounts,
		COUNT(CASE WHEN deleted_at IS NULL AND status = ? THEN 1 END) AS enabled_accounts,
		COUNT(CASE WHEN deleted_at IS NULL AND status = ? THEN 1 END) AS disabled_accounts,
		COUNT(CASE WHEN deleted_at IS NULL AND status = ? AND quota <= 0 THEN 1 END) AS non_positive_wallets,
		COUNT(CASE WHEN deleted_at IS NULL AND status = ? AND quota > 0 AND quota < ? THEN 1 END) AS positive_low_wallets`,
		day.Start, end, common.UserStatusEnabled, common.UserStatusDisabled, common.UserStatusEnabled, common.UserStatusEnabled, result.WalletLowThreshold).Scan(&result).Error
	if err != nil {
		return result, err
	}
	rows, err := db.Table("usage_report_day_rows AS report").
		Select("account.created_at, report.overview").
		Joins("JOIN users AS account ON account.id = report.user_id").
		Where("report.generation = ? AND report.user_id > 0 AND report.overview <> '' AND account.role = ?", day.Generation, common.RoleCommonUser).Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var createdAt int64
		var overview string
		if err := rows.Scan(&createdAt, &overview); err != nil {
			return result, err
		}
		var projection usageReportMetrics
		if err := common.UnmarshalJsonStr(overview, &projection); err != nil {
			return result, err
		}
		metrics := projection.Metrics
		if metrics.TotalCalls > 0 {
			result.AccountsWithCalls++
			if createdAt >= day.Start && createdAt < end {
				result.NewAccountsWithCalls++
			}
		}
		if metrics.FailureCalls > 0 {
			result.AccountsWithFailures++
			if metrics.FailureCalls == metrics.TotalCalls {
				result.AccountsWithOnlyFailures++
			}
		}
	}
	return result, rows.Err()
}
