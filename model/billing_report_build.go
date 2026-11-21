package model

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type BillingReportExplain func(*Log, *CustomerExportRow)

func BuildBillingReportDay(ctx context.Context, start int64, taskID, runnerID string, gate func(context.Context) error, explain BillingReportExplain) error {
	db := usageReportMainDB(ctx).WithContext(ctx)
	buildTime := time.Now()
	now := buildTime.Unix()
	end := start + billingReportDaySeconds - 1
	day := time.Unix(start, 0).In(UsageAnalyticsZone())
	if day.Hour() != 0 || day.Minute() != 0 || day.Second() != 0 || start > now {
		return errors.New("invalid billing report date")
	}
	if gate == nil {
		gate = usageReportBeforeBatch
	} else {
		ctx = context.WithValue(ctx, usageReportBatchGateKey{}, gate)
	}
	upper, err := dailyReportLogUpperBound(ctx)
	if err != nil {
		return err
	}
	generation := billingReportGeneration{OriginalReplayVersion: CustomerOriginalReplayVersion, ID: common.GetUUID(), Start: start, AsOf: now, UpperLogID: upper}
	generation.Incomplete, err = dailyReportKnownLogCleanup(ctx, db, start)
	if err != nil {
		return err
	}
	dependencies, err := dailyReportDependencies(ctx, start, DailyReportBilling)
	if err != nil {
		return err
	}
	month := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location()).Unix()
	discounts, err := loadProviderChannelBillingDiscounts(ctx, month, nil)
	if err != nil {
		return err
	}
	upstreamNames, err := providerBillingChannelNames(ctx)
	if err != nil {
		return err
	}
	upstream := newUpstreamBillingAccumulator(start, end, discounts, 50000)
	upstream.coverage.preferLowestSourceID = true
	customers := make(map[int]*billingReportCustomerBuild)
	projections := make(map[int]*billingReportUpstreamProjection)
	bases := make(map[providerBillingSummaryKey]map[string]int64)
	upstream.observeBasis = func(key providerBillingSummaryKey, basis decimal.Decimal) {
		if bases[key] == nil {
			bases[key] = make(map[string]int64)
		}
		bases[key][basis.String()]++
	}
	var prior billingReportHead
	err = db.Where("start = ?", start).Take(&prior).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var checkpoint *billingReportCheckpoint
	if lowPriorityDailyReport(ctx) {
		checkpoint, err = beginBillingReportCheckpoint(ctx, db, &generation, prior.Generation, dependencies, taskID, runnerID, buildTime)
		if err != nil {
			return err
		}
		now, upper = generation.AsOf, generation.UpperLogID
		buildTime = time.Unix(0, checkpoint.stage.CopyingStartedAt)
		ctx = context.WithValue(ctx, dailyReportCandidateLeaseKey{}, checkpoint)
	} else if err := createDailyReportRows(ctx, db, 1, &generation); err != nil {
		return err
	}
	var previousRows []billingReportUpstreamDay
	if prior.Generation != "" {
		if err := db.Where("generation = ?", prior.Generation).Find(&previousRows).Error; err != nil {
			return err
		}
	}
	for _, row := range previousRows {
		var p billingReportUpstreamProjection
		raw, err := dailyReportPayload(ctx, db, row.Generation, reportBillingUpstreamPayload, row.ChannelID, row.Payload)
		if err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(raw, &p); err != nil {
			return err
		}
		// Keep each published day's URL membership during late-fact rebuilds.
		p.Leaves, p.Tasks = nil, nil
		projections[row.ChannelID] = &p
	}

	pending := make(map[int64]billingReportLine)
	var writes []billingReportLine
	var issueRows int64
	if checkpoint != nil && checkpoint.stage.IssueRows != nil {
		issueRows = *checkpoint.stage.IssueRows
	}
	writeBytes := 0
	const detailBatchBytes = 128 * 1024
	writeBatchSize := dailyReportWriteBatchSize(ctx, 100)
	flushWrites := func() error {
		if len(writes) == 0 {
			return nil
		}
		if err := beforeDailyReportWrite(ctx, len(writes)); err != nil {
			return err
		}
		writeCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
		err := attachPreviousBillingOriginals(db.WithContext(writeCtx), prior.Generation, writes)
		cancel()
		if err != nil {
			return err
		}
		if checkpoint != nil {
			// The preceding read and this candidate commit are separate SQL
			// operations. A pressure change during the read can still stop the write.
			if err := usageReportBeforeBatch(ctx); err != nil {
				return err
			}
		}
		for i := range writes {
			if writes[i].hasDiagnosticIssue() {
				issueRows++
			}
		}
		if checkpoint != nil {
			err = executeDailyReportBatch(ctx, func(ctx context.Context) error {
				return checkpoint.commit(ctx, db, writes, issueRows, hex.EncodeToString(checkpoint.digest.Sum(nil)))
			})
		} else {
			writeCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
			err = db.WithContext(writeCtx).Create(&writes).Error
			cancel()
		}
		writes = writes[:0]
		writeBytes = 0
		return err
	}
	upstream.coverage.onRow = func(detail UpstreamBillingDetailItem, key providerBillingSummaryKey) error {
		line, found := pending[detail.RowId]
		if !found {
			return errors.New("billing report detail has no matching source")
		}
		delete(pending, detail.RowId)
		detail.ChannelName = projections[key.channelId].ChannelName
		line.OriginalExact = detail.OriginalExactQuota
		line.EvidenceTags = billingReportEvidenceTags(detail)
		rootTask := detail.UpstreamTaskId
		detail.UpstreamTaskId = ""
		raw, err := common.Marshal(detail)
		if err != nil {
			return err
		}
		line.Upstream = BillingStatementJSON(raw)
		if checkpoint != nil {
			size := len(line.Customer) + len(line.Upstream) + len(line.URLKey) + len(line.TokenName) + len(line.GroupName) + len(line.Username) +
				len(line.CustomerModel) + len(line.ProviderModel) + len(line.RequestID) + len(line.UpstreamRequestID) + 2*len(rootTask) + 256
			if len(writes) > 0 && writeBytes+size > detailBatchBytes {
				if err := flushWrites(); err != nil {
					return err
				}
			}
			reused, err := checkpoint.observe(ctx, db, line, rootTask)
			if err != nil || reused {
				return err
			}
			writeBytes += size
		}
		if rootTask != "" {
			line.RootTaskRef, err = common.EncryptShortLivedSecretForScope("billing-report-upstream-task", rootTask)
			if err != nil {
				return err
			}
		}
		writes = append(writes, line)
		if len(writes) >= writeBatchSize {
			return flushWrites()
		}
		return nil
	}

	priceDrafts := make(map[string]string)
	var sourceRevision int64
	policy := BillingStatementReadPolicy{UpperLogID: &upper, ReadOnlySource: true, BeforeBatch: gate, BatchTimeout: dailyReportBatchTimeout(ctx),
		ObserveLogRevision: func(revision int64) { sourceRevision += revision }}
	err = scanUpstreamBillingFacts(withDailyReportLogOrder(ctx), UpstreamBillingDetailFilter{Start: start, End: end}, policy, func(log Log, fact billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
		p := projections[log.ChannelId]
		if p == nil {
			channelCtx, stop := context.WithTimeout(ctx, 3*time.Second)
			channels, err := fetchBillingURLChannelsById(db.WithContext(channelCtx), []int{log.ChannelId})
			stop()
			if err != nil {
				return err
			}
			channel, found := channels[log.ChannelId]
			key, _ := billingURLGroupIdentity(channel, found, log.ChannelId)
			p = &billingReportUpstreamProjection{ChannelID: log.ChannelId, ChannelName: channel.Name, Found: found, URLKey: key}
			projections[log.ChannelId] = p
		}
		if p.BuiltAt != buildTime.UnixNano() {
			p.BuiltAt = buildTime.UnixNano()
			p.UpstreamName = upstreamNames[log.ChannelId]
			if discount, known := providerChannelBillingDiscountFor(discounts, month, log.ChannelId); known {
				p.Discount = &discount
			} else {
				p.Discount = nil
			}
		}
		line := billingReportLine{Generation: generation.ID, SourceID: int64(log.Id), UserID: log.UserId, ChannelID: log.ChannelId,
			TokenID: log.TokenId, LogType: log.Type, CreatedAt: log.CreatedAt, RequestID: log.RequestId, UpstreamRequestID: log.UpstreamRequestId,
			CustomerModel: parsed.customerModel, ProviderModel: strings.TrimSpace(parsed.providerModel), BillingMode: parsed.billingMode,
			TokenName: log.TokenName, GroupName: log.Group, URLKey: p.URLKey, Username: log.Username}
		line.PriceIssue = historicalImagePriceIssue(log, parsed, priceDrafts)
		line.ReplayIssue = parsed.originalReplayReason
		if line.ProviderModel == "" {
			line.ProviderModel, line.ModelFallback = fact.ModelName, true
		}
		if log.UserId > 0 && !parsed.isChannelTest {
			c := customers[log.UserId]
			if c == nil {
				c = newBillingReportCustomerBuild()
				customers[log.UserId] = c
			}
			if err := c.observe(fact, parsed, int64(log.Id)); err != nil {
				return err
			}
			line.FundGroup = billingStatementFundIdentity(log, parsed)
			export := buildCustomerExportRow(customerBillingLogScanRow(&log), fact, parsed)
			if explain != nil {
				explain(&log, &export)
			}
			record := billingReportCustomerLine{Export: export, UseTime: log.UseTime, IsStream: log.IsStream, RecordedInput: parsed.recordedInputTokens}
			raw, err := common.Marshal(record)
			if err != nil {
				return err
			}
			line.Customer = BillingStatementJSON(raw)
			line.CustomerCharge = max(export.Quota, 0)
			if line.LogType == LogTypeRefund {
				line.CustomerCharge = -line.CustomerCharge
			}
			line.BillableRequests, line.InputTokens, line.OutputTokens = export.RequestCount, export.InputTokens, export.OutputTokens
		}
		pending[line.SourceID] = line
		generation.Rows++
		return upstream.observe(ctx, log, fact, parsed)
	})
	if err != nil {
		return err
	}
	if err := upstream.coverage.flush(ctx, upstream.items); err != nil {
		return err
	}
	if len(pending) != 0 {
		return errors.New("billing report source has no matching detail")
	}
	if err := flushWrites(); err != nil {
		return err
	}
	coverage := dailyReportLogScanCoverage(ctx, generation.Rows, sourceRevision)
	if checkpoint != nil {
		if err := checkpoint.prepareSummaries(ctx, db, coverage); err != nil {
			return err
		}
	}
	for key, item := range upstream.items {
		p := projections[key.channelId]
		p.Leaves = append(p.Leaves, billingReportUpstreamLeaf{RecordedOriginalBasis: true, Model: key.model, Mode: key.mode, Fallback: key.fallback, Usage: item.Usage, Quality: item.DataQuality,
			Reasons: item.EstimateReasons, Models: item.customerModels, Original: item.originalQuota, Reference: item.referenceQuota, Known: item.originalQuotaKnown,
			Complete: item.originalComplete, TestRows: item.testMoneyRows, MoneyRows: item.moneyRows, SettlementRows: item.settlementRows, Basis: bases[key]})
	}
	for key, bucket := range upstream.taskCache.buckets {
		p := projections[key.item.channelId]
		p.Tasks = append(p.Tasks, billingReportUpstreamTask{Model: key.item.model, Mode: key.item.mode, Fallback: key.item.fallback, UserID: key.userID, TaskID: key.taskID, Read: bucket.read, Write: bucket.write})
	}
	for key, recovery := range upstream.coverage.recovered {
		p := projections[key.item.channelId]
		p.Tasks = append(p.Tasks, billingReportUpstreamTask{Model: key.item.model, Mode: key.item.mode, Fallback: key.item.fallback, UserID: key.task.userID, TaskID: key.task.taskID, Recovered: &recovery})
	}
	for id, p := range projections {
		if len(p.Leaves) == 0 {
			continue
		}
		if checkpoint != nil {
			reused, err := checkpoint.reuseSummary(ctx, db, &billingReportUpstreamDay{}, "channel_id", reportBillingUpstreamPayload, id)
			if err != nil {
				return err
			}
			if reused {
				continue
			}
		}
		raw, err := common.Marshal(p)
		if err != nil {
			return err
		}
		payload, err := writeDailyReportPayload(ctx, db, generation.ID, reportBillingUpstreamPayload, id, raw)
		if err != nil {
			return err
		}
		if err := writeBillingReportProjection(ctx, db, len(payload), &billingReportUpstreamDay{Generation: generation.ID, ChannelID: id, Payload: payload}); err != nil {
			return err
		}
	}
	for id, customer := range customers {
		if checkpoint != nil {
			reused, err := checkpoint.reuseSummary(ctx, db, &billingReportCustomerDay{}, "user_id", reportBillingCustomerPayload, id)
			if err != nil {
				return err
			}
			if reused {
				continue
			}
		}
		if err := writeBillingReportCustomerDay(ctx, db, generation.ID, id, customer); err != nil {
			return err
		}
	}
	if err := gate(ctx); err != nil {
		return err
	}
	stage := dailyReportStage{OriginalReplayVersion: generation.OriginalReplayVersion, Start: start, Kind: DailyReportBilling, Generation: generation.ID,
		Previous: prior.Generation, AsOf: now, UpperLogID: upper, SourceRows: coverage.Rows, SourceRevision: coverage.Revision, Dependencies: dependencies,
		IssueRows: &issueRows, IssueRowsGeneration: generation.ID, Incomplete: generation.Incomplete}
	if prior.Generation == "" && generation.Incomplete && coverage.Rows == 0 {
		stage.HistoricalSource = dailyReportSourceMissing
	}
	return saveDailyReportStage(ctx, db, stage, taskID, runnerID)
}

func writeBillingReportProjection(ctx context.Context, db *gorm.DB, size int, row any) error {
	if size > 2*1024*1024 {
		return fmt.Errorf("billing report projection exceeds size budget: %d bytes", size)
	}
	return createDailyReportRows(ctx, db, 1, row)
}
