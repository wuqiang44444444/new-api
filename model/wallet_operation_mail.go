package model

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// WalletOperation is the committed fact for manual wallet changes and top-up
// completions. It is separate from best-effort audit and usage logs.
type WalletOperation struct {
	ID        int64 `gorm:"primaryKey"`
	UserID    int
	ActorID   int
	Kind      string `gorm:"type:varchar(24)"`
	Before    int    `gorm:"column:quota_before"`
	After     int    `gorm:"column:quota_after"`
	CreatedAt int64  `gorm:"type:bigint;index"`
}

type WalletMailBuilder func(tx *gorm.DB, operation WalletOperation) error

func RecordWalletOperation(tx *gorm.DB, operation WalletOperation, build WalletMailBuilder) error {
	if operation.Before == operation.After {
		return nil
	}
	if err := tx.Create(&operation).Error; err != nil {
		return err
	}
	if build != nil {
		return build(tx, operation)
	}
	return nil
}

func FinanceMailRecipients(tx *gorm.DB) ([]string, error) {
	var row Option
	result := tx.Session(&gorm.Session{Logger: logger.Discard}).Where(&Option{Key: financeMailRecipientsOption}).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		row.Value = os.Getenv("FINANCE_MAIL_RECIPIENTS")
	}
	return operation_setting.ValidateErrorReportRecipients(row.Value)
}

type WalletOperationSummary struct {
	Kind     string
	Count    int64
	Credited decimal.Decimal
	Debited  decimal.Decimal
}

func ReadWalletOperationsSummary(ctx context.Context, start, end int64) ([]WalletOperationSummary, error) {
	rows, err := DB.WithContext(ctx).Model(&WalletOperation{}).Where("created_at >= ? AND created_at < ?", start, end).
		Select("kind, quota_after - quota_before").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := make(map[string]*WalletOperationSummary)
	for rows.Next() {
		var kind string
		var delta int64
		if err := rows.Scan(&kind, &delta); err != nil {
			return nil, err
		}
		summary := summaries[kind]
		if summary == nil {
			summary = &WalletOperationSummary{Kind: kind}
			summaries[kind] = summary
		}
		summary.Count++
		amount := decimal.NewFromInt(delta)
		if delta > 0 {
			summary.Credited = summary.Credited.Add(amount)
		} else {
			summary.Debited = summary.Debited.Sub(amount)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]WalletOperationSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, *summary)
	}
	slices.SortFunc(result, func(a, b WalletOperationSummary) int { return strings.Compare(a.Kind, b.Kind) })
	return result, nil
}

const (
	WalletReceiptMail = "wallet_receipt"
	WalletRiskMail    = "wallet_risk"
	WalletSummaryMail = "wallet_summary"
)

// MailUserEmail only selects the account address. Financial receipts do not
// use the general-purpose notification destination.
func MailUserEmail(db *gorm.DB, userID int) (string, error) {
	var user User
	if err := db.Select("email").First(&user, userID).Error; err != nil {
		return "", err
	}
	return user.Email, nil
}
