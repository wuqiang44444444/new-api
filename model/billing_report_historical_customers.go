package model

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Build only the missing projections from frozen line evidence. A frozen
// summary can cover records whose individual details have already been lost.
func rebuildMissingHistoricalBillingCustomers(ctx context.Context, db *gorm.DB, generation string) error {
	customers := make(map[int]*billingReportCustomerBuild)
	var cursor int64
	for {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		var rows []billingReportLine
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(readCtx).Select(`source_id, user_id, token_id, channel_id, created_at, log_type, replay_issue,
			CASE WHEN user_id <= 0 OR EXISTS (SELECT 1 FROM billing_report_customer_days d
			WHERE d.generation = billing_report_lines.generation AND d.user_id = billing_report_lines.user_id)
			THEN '' ELSE customer END AS customer`).
			Where("generation = ? AND source_id > ?", generation, cursor).
			Order("source_id").Limit(500).Find(&rows).Error
		cancel()
		if err != nil {
			return err
		}
		for _, line := range rows {
			cursor = line.SourceID
			if line.Customer == "" {
				continue
			}
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
				frozenBillableSeconds: row.BillableSeconds, frozenSecondsMissing: row.BillableSecondsMissing,
				customerModel: row.CustomerModel, billingMode: row.BillingMode, requestCount: row.RequestCount,
				isRequest: row.RequestCount > 0, isRefund: row.EventType == "refund",
				inputTokens: row.InputTokens, outputTokens: row.OutputTokens, inputTokensUnavailable: row.InputTokensUnavailable,
				cacheReadTokens: row.CacheReadTokens, cacheWrite: billingStatementCacheWriteTokens{total: row.CacheWriteTokens, tiers: row.CacheWrites},
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
		if err := discardIncompleteHistoricalCustomerPayload(ctx, db, generation, userID); err != nil {
			return err
		}
		if err := writeBillingReportCustomerDay(ctx, db, generation, userID, customer); err != nil {
			return err
		}
	}
	return nil
}

// A customer row marks the whole projection complete. When it is absent,
// discard orphan fragments for just this customer before regenerating it.
func discardIncompleteHistoricalCustomerPayload(ctx context.Context, db *gorm.DB, generation string, userID int) error {
	for {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		var parts []dailyReportPayloadPart
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(readCtx).Select("generation", "kind", "scope_id", "part").
			Where("generation = ? AND kind = ? AND scope_id = ?", generation, reportBillingCustomerPayload, userID).Limit(4).Find(&parts).Error
		cancel()
		if err != nil || len(parts) == 0 {
			return err
		}
		if err := beforeDailyReportWrite(ctx, len(parts)); err != nil {
			return err
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.WithContext(writeCtx).Delete(&parts).Error
		cancel()
		if err != nil || len(parts) < 4 {
			return err
		}
	}
}

func writeBillingReportCustomerDay(ctx context.Context, db *gorm.DB, generation string, userID int, customer *billingReportCustomerBuild) error {
	summary, err := common.Marshal(billingReportCustomerOverview{projectBillingReportPrice(&customer.overview), customer.lastActivityAt})
	if err != nil {
		return err
	}
	leaves, err := common.Marshal(billingReportCustomerDetails{projectBillingReportDimension(customer.key), projectBillingReportDimension(customer.channel)})
	if err != nil {
		return err
	}
	payload, err := writeDailyReportPayload(ctx, db, generation, reportBillingCustomerPayload, userID, leaves)
	if err != nil {
		return err
	}
	row := billingReportCustomerDay{Generation: generation, UserID: userID, Summary: BillingStatementJSON(summary), Leaves: payload}
	return writeBillingReportProjection(ctx, db, len(summary)+len(payload), &row)
}
