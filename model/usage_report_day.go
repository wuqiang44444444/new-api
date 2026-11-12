package model

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const UsageReportTaskType = "usage_report_day"

const usageReportMeterVersion = 1

type usageReportDay struct {
	Start        int64  `gorm:"primaryKey;autoIncrement:false"`
	Generation   string `gorm:"size:32;not null"`
	AsOf         int64
	SourceRows   int64
	UpperLogID   int64
	Incomplete   bool
	RankReady    bool
	MeterVersion int
}

type usageReportDayRow struct {
	Generation    string `gorm:"size:32;primaryKey"`
	UserID        int    `gorm:"primaryKey;autoIncrement:false;index"`
	Payload       BillingStatementJSON
	Overview      BillingStatementJSON
	CreatedAt     int64 `gorm:"index"`
	RetiredAt     int64 `gorm:"index"`
	UnavailableAt int64
}

type usageReportCustomerLeaf struct {
	Token       int
	Model       string
	BillingMode string
	Metrics     usageReportMetrics
}

type usageReportUpstreamLeaf struct {
	Channel  int
	Model    string
	Mode     string
	Fallback bool
	URLKey   string
	URLName  string
	Name     string
	Metrics  usageReportMetrics
}

type PublishedUsagePendingRefund struct {
	TaskID      string `json:"task_id"`
	RequestedAt int64  `json:"requested_at"`
}

type usageReportDayData struct {
	PendingRefunds []PublishedUsagePendingRefund
	Customers      []usageReportCustomerLeaf
	Upstream       []usageReportUpstreamLeaf
	Discounts      map[int64]map[int]ProviderChannelBillingDiscount
}

type UsageReportDayReference struct {
	Start      int64  `json:"start"`
	Generation string `json:"generation"`
}

type PublishedUsageReport struct {
	References     []UsageReportDayReference     `json:"report_days"`
	IncompleteDays []string                      `json:"incomplete_days,omitempty"`
	PublishedDays  []string                      `json:"published_days"`
	PendingRefunds []PublishedUsagePendingRefund `json:"pending_refunds,omitempty"`
	AsOf           int64                         `json:"as_of"`
}

func BuildUsageReportDay(ctx context.Context, day string, taskID string, runnerID string, beforeBatch func(context.Context) error) error {
	now := common.GetTimestamp()
	db := usageReportMainDB(ctx).WithContext(ctx)
	period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, day, now)
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, usageReportBatchGateKey{}, beforeBatch)
	upper, err := dailyReportLogUpperBound(ctx)
	if err != nil {
		return err
	}
	opts := usageViewOptions{WantCustomer: true, WantUpstream: true, LogUpper: &upper}
	incomplete, err := dailyReportKnownLogCleanup(ctx, db, period.StartTimestamp)
	if err != nil {
		return err
	}
	dependencies, err := dailyReportDependencies(ctx, period.StartTimestamp, DailyReportUsage)
	if err != nil {
		return err
	}
	agg, err := runUsageAggregation(ctx, period, opts)
	if err != nil {
		return err
	}

	data := make(map[int]*usageReportDayData)
	overview := make(map[int]*usageMetricsAcc)
	modelTotals := make(map[usageReportModelKey]*usageMetricsAcc)
	for key, acc := range agg.customer {
		if key.user <= 0 {
			continue
		}
		if data[key.user] == nil {
			data[key.user] = &usageReportDayData{}
			overview[key.user] = &usageMetricsAcc{}
		}
		data[key.user].Customers = append(data[key.user].Customers, usageReportCustomerLeaf{
			Token: key.token, Model: key.model, BillingMode: key.billingMode, Metrics: makeUsageReportMetrics(acc),
		})
		overview[key.user].merge(acc)
		mergeUsageReportModel(modelTotals, usageReportModelKey{key.model, key.billingMode}, acc)
	}
	for _, task := range agg.tasks {
		if task.UserID <= 0 || task.TaskID == "" || task.VideoRefundState != "pending" ||
			task.VideoRefundRequestedAt <= 0 || task.VideoRefundCompletedAt > 0 ||
			now-task.VideoRefundRequestedAt <= overdueBillingRefundSeconds {
			continue
		}
		if data[task.UserID] == nil {
			data[task.UserID] = &usageReportDayData{}
		}
		data[task.UserID].PendingRefunds = append(data[task.UserID].PendingRefunds, PublishedUsagePendingRefund{
			TaskID: task.TaskID, RequestedAt: task.VideoRefundRequestedAt,
		})
	}
	upstream := &usageReportDayData{Discounts: agg.bundle.byMonth}
	data[0] = upstream
	channelIDs := make([]int, 0, len(agg.upstream))
	seen := make(map[int]bool)
	for key := range agg.upstream {
		if !seen[key.channel] {
			seen[key.channel] = true
			channelIDs = append(channelIDs, key.channel)
		}
	}
	channels, err := fetchBillingURLChannelsById(db, channelIDs)
	if err != nil {
		return err
	}
	// A correction preserves the published channel mapping for that day.
	var prior usageReportDayRow
	err = db.Table("usage_report_day_rows AS detail").Select("detail.generation, detail.payload").
		Joins("JOIN usage_report_days AS head ON head.generation = detail.generation").
		Where("head.start = ? AND detail.user_id = ?", period.StartTimestamp, 0).Take(&prior).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var priorData usageReportDayData
	if prior.Generation != "" {
		raw, err := dailyReportPayload(ctx, db, prior.Generation, reportUsagePayload, 0, prior.Payload)
		if err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(raw, &priorData); err != nil {
			return err
		}
	}
	priorChannels := make(map[int]usageReportUpstreamLeaf)
	for _, leaf := range priorData.Upstream {
		priorChannels[leaf.Channel] = leaf
	}
	for key, acc := range agg.upstream {
		channel, found := channels[key.channel]
		urlKey, urlName := billingURLGroupIdentity(channel, found, key.channel)
		name := channel.Name
		if leaf, exists := priorChannels[key.channel]; exists {
			urlKey, urlName, name = leaf.URLKey, leaf.URLName, leaf.Name
		}
		upstream.Upstream = append(upstream.Upstream, usageReportUpstreamLeaf{
			Channel: key.channel, Model: key.model, Mode: key.mode, Fallback: key.fallback,
			URLKey: urlKey, URLName: urlName, Name: name, Metrics: makeUsageReportMetrics(acc),
		})
	}

	generation := common.GetUUID()
	if err := db.Create(&usageReportDayRow{Generation: generation, UserID: -1}).Error; err != nil {
		return err
	}
	if err := writeUsageReportModels(ctx, db, generation, modelTotals); err != nil {
		return err
	}
	if err := writeUsageReportRanks(ctx, db, generation, modelTotals, upstream.Upstream); err != nil {
		return err
	}
	ids := make([]int, 0, len(data))
	for id := range data {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for start := 0; start < len(ids); start += 50 {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		rows := make([]usageReportDayRow, 0, 50)
		bytes := 0
		for _, id := range ids[start:min(start+50, len(ids))] {
			encoded, err := common.Marshal(data[id])
			if err != nil {
				return err
			}
			payload, err := writeDailyReportPayload(ctx, db, usageReportBeforeBatch, generation, reportUsagePayload, id, encoded)
			if err != nil {
				return err
			}
			row := usageReportDayRow{Generation: generation, UserID: id, Payload: payload}
			if acc := overview[id]; acc != nil {
				projected, err := common.Marshal(makeUsageReportMetrics(acc))
				if err != nil {
					return err
				}
				row.Overview = BillingStatementJSON(projected)
			}
			size := len(payload) + len(row.Overview)
			if len(rows) > 0 && bytes+size > 2*1024*1024 {
				if err := db.Create(&rows).Error; err != nil {
					return err
				}
				rows = rows[:0]
				bytes = 0
			}
			rows = append(rows, row)
			bytes += size
		}
		if err := db.Create(&rows).Error; err != nil {
			return err
		}
	}
	if err := usageReportBeforeBatch(ctx); err != nil {
		return err
	}
	coverage := dailyReportLogScanCoverage(ctx, agg.scannedLogRows, agg.logRevision)
	stage := dailyReportStage{OriginalReplayVersion: CustomerOriginalReplayVersion, UsageMeterVersion: usageReportMeterVersion, Start: period.StartTimestamp, Kind: DailyReportUsage, Generation: generation,
		Previous: prior.Generation, AsOf: now, UpperLogID: upper, SourceRows: coverage.Rows, SourceRevision: coverage.Revision, Dependencies: dependencies, Incomplete: incomplete, RankReady: true}
	return saveDailyReportStage(ctx, db, stage, taskID, runnerID)
}

type usageReportBatchGateKey struct{}
type usageReportReferencesKey struct{}

func WithUsageReportReferences(ctx context.Context, refs []UsageReportDayReference) context.Context {
	return context.WithValue(ctx, usageReportReferencesKey{}, refs)
}

func reportDayHeads(ctx context.Context, period UsageAnalyticsPeriod, refs []UsageReportDayReference) ([]usageReportDay, error) {
	var heads []usageReportDay
	if len(refs) == 0 {
		err := DB.WithContext(ctx).Where("start >= ? AND start < ?", period.StartTimestamp, period.EndTimestamp).Find(&heads).Error
		return heads, err
	}
	if len(refs) > len(period.Days) {
		return nil, errors.New("invalid usage report versions")
	}
	days := make(map[int64]bool)
	gens := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Start < period.StartTimestamp || ref.Start >= period.EndTimestamp || len(ref.Generation) != 32 || days[ref.Start] {
			return nil, errors.New("invalid usage report versions")
		}
		days[ref.Start] = true
		gens = append(gens, ref.Generation)
	}
	var rows []usageReportDayRow
	if err := DB.WithContext(ctx).Where("generation IN ? AND user_id = -1 AND payload <> ? AND COALESCE(unavailable_at, 0) = 0", gens, "").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(refs) {
		return nil, errors.New("usage report version is unavailable")
	}
	versions := make(map[int64]string, len(refs))
	for _, ref := range refs {
		versions[ref.Start] = ref.Generation
	}
	for _, row := range rows {
		var head usageReportDay
		if err := common.UnmarshalJsonStr(string(row.Payload), &head); err != nil {
			return nil, err
		}
		if versions[head.Start] != row.Generation || head.Generation != row.Generation {
			return nil, errors.New("invalid usage report version for the selected date")
		}
		heads = append(heads, head)
	}
	return heads, nil
}

func FreezeUsageReportReferences(ctx context.Context, period UsageAnalyticsPeriod, requested []UsageReportDayReference) ([]UsageReportDayReference, error) {
	heads, err := reportDayHeads(ctx, period, requested)
	if err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		now := common.GetTimestamp()
		heads = slices.DeleteFunc(heads, func(day usageReportDay) bool {
			return day.Start+86400 <= now && day.AsOf < day.Start+86400+3*3600
		})
	}
	if len(heads) == 0 {
		return nil, errors.New("usage report has not been published")
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].Start < heads[j].Start })
	refs := make([]UsageReportDayReference, 0, len(heads))
	for _, head := range heads {
		refs = append(refs, UsageReportDayReference{Start: head.Start, Generation: head.Generation})
	}
	return refs, nil
}

func usageReportBeforeBatch(ctx context.Context) error {
	if gate, ok := ctx.Value(usageReportBatchGateKey{}).(func(context.Context) error); ok && gate != nil {
		return gate(ctx)
	}
	return ctx.Err()
}

func loadPublishedUsageReport(ctx context.Context, period UsageAnalyticsPeriod, userID int, upstream, overview bool) (*usageAggregation, PublishedUsageReport, error) {
	result := PublishedUsageReport{PublishedDays: []string{}, References: []UsageReportDayReference{}}
	agg := newUsageAggregation(period, usageViewOptions{UserID: userID, WantCustomer: !upstream && !overview, WantUpstream: upstream, WantOverview: overview})
	agg.bundle = &usageDiscountBundle{byMonth: make(map[int64]map[int]ProviderChannelBillingDiscount)}
	refs, _ := ctx.Value(usageReportReferencesKey{}).([]UsageReportDayReference)
	heads, err := reportDayHeads(ctx, period, refs)
	if err != nil {
		return nil, result, err
	}
	var issues map[int64]bool
	if len(refs) == 0 && len(heads) > 0 {
		issues, err = dailyReportCoverageIssues(ctx, DailyReportUsage, period.StartTimestamp, period.EndTimestamp)
		if err != nil {
			return nil, result, err
		}
	}
	byDay := make(map[int64]usageReportDay, len(heads))
	for _, head := range heads {
		byDay[head.Start] = head
	}
	for index, day := range period.Days {
		head, ok := byDay[day.Start]
		if !ok || day.Future || (len(refs) == 0 && day.End <= common.GetTimestamp() && head.AsOf < day.End+3*3600) {
			continue
		}
		if head.Incomplete || issues[day.Start] {
			result.IncompleteDays = append(result.IncompleteDays, day.Date)
		}
		result.PublishedDays = append(result.PublishedDays, day.Date)
		result.References = append(result.References, UsageReportDayReference{Start: day.Start, Generation: head.Generation})
		if result.AsOf == 0 || head.AsOf < result.AsOf {
			result.AsOf = head.AsOf
		}
		query := DB.WithContext(ctx).Model(&usageReportDayRow{}).Where("generation = ?", head.Generation)
		if overview {
			query = query.Select("user_id, overview").Where("user_id > 0")
		} else {
			query = query.Where("user_id = ?", userID)
		}
		var rows []usageReportDayRow
		if err := query.Find(&rows).Error; err != nil {
			return nil, result, err
		}
		for _, row := range rows {
			if overview {
				if len(row.Overview) == 0 {
					continue
				}
				var metrics usageReportMetrics
				if err := common.UnmarshalJsonStr(string(row.Overview), &metrics); err != nil {
					return nil, result, err
				}
				agg.overview[usageOverviewKey{user: row.UserID, day: index}] = metrics.accumulator()
				continue
			}
			var data usageReportDayData
			raw, err := dailyReportPayload(ctx, DB, row.Generation, reportUsagePayload, row.UserID, row.Payload)
			if err != nil {
				return nil, result, err
			}
			if err := common.UnmarshalJsonStr(raw, &data); err != nil {
				return nil, result, err
			}
			if agg.opts.WantCustomer {
				result.PendingRefunds = append(result.PendingRefunds, data.PendingRefunds...)
			}
			for _, leaf := range data.Customers {
				if agg.opts.WantCustomer {
					agg.customer[usageCustomerKey{day: index, token: leaf.Token, model: leaf.Model, billingMode: leaf.BillingMode}] = leaf.Metrics.accumulator()
				}
			}
			for _, leaf := range data.Upstream {
				if upstream {
					key := usageUpstreamKey{day: index, channel: leaf.Channel, model: leaf.Model, mode: leaf.Mode, fallback: leaf.Fallback, urlKey: leaf.URLKey, urlName: leaf.URLName, name: leaf.Name}
					agg.upstream[key] = leaf.Metrics.accumulator()
				}
			}
			for month, coefficients := range data.Discounts {
				agg.bundle.byMonth[month] = coefficients
			}
		}
	}
	return agg, result, nil
}

func GetPublishedUsageCustomer(ctx context.Context, period UsageAnalyticsPeriod, userID int, modelName string) (UsageCustomerView, PublishedUsageReport, error) {
	return getPublishedUsageCustomer(ctx, period, userID, modelName, false)
}

func GetPublishedAdminUsageCustomer(ctx context.Context, period UsageAnalyticsPeriod, userID int) (UsageCustomerView, PublishedUsageReport, error) {
	return getPublishedUsageCustomer(ctx, period, userID, "", true)
}

func getPublishedUsageCustomer(ctx context.Context, period UsageAnalyticsPeriod, userID int, modelName string, includeModelRanks bool) (UsageCustomerView, PublishedUsageReport, error) {
	view := UsageCustomerView{Keys: []UsageCustomerKeyGroup{}}
	agg, meta, err := loadPublishedUsageReport(ctx, period, userID, false, false)
	if err == nil {
		if modelName != "" {
			for key := range agg.customer {
				if key.model != modelName {
					delete(agg.customer, key)
				}
			}
		}
		finalizeUsageCustomerView(&view, agg)
		if includeModelRanks {
			modelTotals := make(map[usageReportModelKey]*usageMetricsAcc)
			for key, acc := range agg.customer {
				mergeUsageReportModel(modelTotals, usageReportModelKey{key.model, key.billingMode}, acc)
			}
			view.ModelRanks = make([]UsageReportRankItem, 0, len(modelTotals))
			for key, acc := range modelTotals {
				metrics := acc.finalize()
				item := usageReportCustomerRankItem(key, metrics)
				switch key.mode {
				case BillingReconciliationModeToken:
					item.TokenUsage = &UsageReportRankTokenUsage{
						InputTokens: metrics.InputTokens, OutputTokens: metrics.OutputTokens,
						CacheReadTokens: metrics.CacheReadTokens, CacheWriteTokens: metrics.CacheWriteTokens,
						TokenDetails: metrics.TokenDetails,
					}
				case BillingReconciliationModeUnknown:
					item.MissingTokens = metrics.RowsMissingTokens
				}
				view.ModelRanks = append(view.ModelRanks, item)
			}
		}
	}
	return view, meta, err
}

func GetPublishedUsageOverview(ctx context.Context, period UsageAnalyticsPeriod, search string) (UsageCustomersOverview, PublishedUsageReport, error) {
	view := UsageCustomersOverview{Customers: []UsageCustomerOverviewRow{}}
	agg, meta, err := loadPublishedUsageReport(ctx, period, 0, false, true)
	if err == nil {
		err = finalizeUsageCustomersOverview(ctx, &view, agg, search)
	}
	return view, meta, err
}

func GetPublishedUsageUpstream(ctx context.Context, period UsageAnalyticsPeriod, scope UsageUpstreamScope) (UsageUpstreamView, PublishedUsageReport, error) {
	view := UsageUpstreamView{UrlGroups: []UsageUpstreamUrlGroup{}}
	agg, meta, err := loadPublishedUsageReport(ctx, period, 0, true, false)
	if err == nil {
		err = finalizeUsageUpstreamView(ctx, &view, agg, scope)
	}
	return view, meta, err
}

// PruneUsageReportRows removes only unpublished or superseded generations
// after the export job retention period, in small writes.
func PruneUsageReportRows(ctx context.Context, now int64) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	cutoff := now - 8*86400
	var marker usageReportDayRow
	err := db.Where("user_id = -1 AND ((retired_at > 0 AND retired_at < ?) OR (payload = ? AND created_at < ?))", cutoff, "", cutoff).
		Where("COALESCE(unavailable_at, 0) < ?", now-60).
		Where("NOT EXISTS (SELECT 1 FROM daily_report_stages stage WHERE stage.generation = usage_report_day_rows.generation AND stage.as_of >= ?)", cutoff).Order("created_at").Take(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(marker.Payload) > 0 && marker.UnavailableAt == 0 {
		return false, db.Transaction(func(tx *gorm.DB) error {
			if err := lockForUpdate(tx).Where("generation = ? AND user_id = -1", marker.Generation).Take(&marker).Error; err != nil {
				return err
			}
			var active int64
			if err := activeDailyReportExports(tx, marker.Generation).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return nil
			}
			return tx.Model(&marker).Update("unavailable_at", now).Error
		})
	}
	var userIDs []int
	if err := db.Model(&usageReportDayRow{}).Where("generation = ? AND user_id <> -1", marker.Generation).
		Limit(200).Pluck("user_id", &userIDs).Error; err != nil {
		return false, err
	}
	if len(userIDs) == 0 {
		userIDs = []int{-1}
	}
	if len(userIDs) == 1 && userIDs[0] == -1 {
		var rankIDs []int64
		if err := db.Model(&usageReportRank{}).Where("generation = ?", marker.Generation).Limit(500).Pluck("id", &rankIDs).Error; err != nil {
			return false, err
		}
		if len(rankIDs) > 0 {
			return true, db.Where("id IN ?", rankIDs).Delete(&usageReportRank{}).Error
		}
		if pruned, err := pruneDailyReportPayload(ctx, db, marker.Generation); err != nil || pruned {
			return pruned, err
		}
		return true, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("generation = ?", marker.Generation).Delete(&dailyReportExportReference{}).Error; err != nil {
				return err
			}
			return tx.Delete(&marker).Error
		})
	}
	return true, db.Where("generation = ? AND user_id IN ?", marker.Generation, userIDs).Delete(&usageReportDayRow{}).Error
}
