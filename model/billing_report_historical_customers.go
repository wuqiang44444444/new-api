package model

import (
	"context"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Build only the missing projections from frozen line evidence. A frozen
// summary can cover records whose individual details have already been lost.
func rebuildMissingHistoricalBillingCustomers(ctx context.Context, tx *gorm.DB, generation string) error {
	customers := make(map[int]*billingReportCustomerBuild)
	var cursor int64
	for {
		var rows []billingReportLine
		err := tx.Where("generation = ? AND user_id > 0 AND source_id > ? AND customer <> ''", generation, cursor).
			Where("NOT EXISTS (SELECT 1 FROM billing_report_customer_days d WHERE d.generation = billing_report_lines.generation AND d.user_id = billing_report_lines.user_id)").
			Order("source_id").Limit(500).Find(&rows).Error
		if err != nil {
			return err
		}
		for _, line := range rows {
			cursor = line.SourceID
			detail, err := line.customerDetail()
			if err != nil {
				return err
			}
			row := detail.Export
			if !row.CountsTowardBill {
				continue
			}
			customer := customers[line.UserID]
			if customer == nil {
				customer = newBillingReportCustomerBuild()
				customers[line.UserID] = customer
			}
			log := billingReconciliationLog{UserId: line.UserID, TokenId: line.TokenID, TokenName: row.TokenName, ChannelId: line.ChannelID,
				ModelName: row.CustomerModel, Type: line.LogType, CreatedAt: line.CreatedAt, Quota: int(row.Quota)}
			facts := parsedBillingReconciliationLog{
				customerModel: row.CustomerModel, billingMode: row.BillingMode, requestCount: row.RequestCount,
				isRequest: row.RequestCount > 0, isRefund: row.EventType == "refund",
				inputTokens: row.InputTokens, outputTokens: row.OutputTokens, inputTokensUnavailable: row.InputTokensUnavailable,
				cacheReadTokens: row.CacheReadTokens, cacheWrite: billingStatementCacheWriteTokens{total: row.CacheWriteTokens},
				billableImageApplicable: row.BillingMode == BillingReconciliationModePerImage, billableImageCount: row.BillableImageCount,
				discountRatio: row.GroupRatio, groupName: row.GroupName, groupRatioSource: row.GroupRatioSource,
				contractDiscountRatio: row.ContractRatio, contractApplicable: row.ContractApplicable == "yes",
				contractApplicableKnown: row.ContractApplicable == "yes" || row.ContractApplicable == "no",
				originalReplayReason:    line.ReplayIssue, hasAuxiliaryCharge: row.HasAuxiliaryCharge,
			}
			if quota, err := strconv.ParseInt(row.RecordedOriginal, 10, 64); err == nil {
				if facts.isRefund {
					quota = -quota
				}
				if quota >= 0 {
					facts.recordedOriginalQuota = &quota
				}
			}
			if err := customer.observe(log, facts, line.SourceID); err != nil {
				return err
			}
		}
		if len(rows) < 500 {
			break
		}
	}
	for userID, customer := range customers {
		if err := writeBillingReportCustomerDay(ctx, tx, usageReportBeforeBatch, generation, userID, customer); err != nil {
			return err
		}
	}
	return nil
}

func writeBillingReportCustomerDay(ctx context.Context, db *gorm.DB, gate func(context.Context) error, generation string, userID int, customer *billingReportCustomerBuild) error {
	summary, err := common.Marshal(billingReportCustomerOverview{projectBillingReportPrice(&customer.overview), customer.lastActivityAt})
	if err != nil {
		return err
	}
	leaves, err := common.Marshal(billingReportCustomerDetails{projectBillingReportDimension(customer.key), projectBillingReportDimension(customer.channel)})
	if err != nil {
		return err
	}
	payload, err := writeDailyReportPayload(ctx, db, gate, generation, reportBillingCustomerPayload, userID, leaves)
	if err != nil {
		return err
	}
	row := billingReportCustomerDay{Generation: generation, UserID: userID, Summary: BillingStatementJSON(summary), Leaves: payload}
	return writeBillingReportProjection(ctx, db, gate, len(summary)+len(payload), &row)
}
