package model

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type billingReportUpstreamLeaf struct {
	RecordedOriginalBasis bool
	Model                 string
	Mode                  string
	Fallback              bool
	Usage                 ProviderBillingUsage
	Quality               *BillingReconciliationDataQuality
	Reasons               []string
	Models                map[string]struct{}
	Original              decimal.Decimal
	Reference             decimal.Decimal
	Known                 bool
	Complete              bool
	TestRows              int
	MoneyRows             int
	SettlementRows        int
	Basis                 map[string]int64
}

func (l billingReportUpstreamLeaf) key(channelID int) providerBillingSummaryKey {
	return providerBillingSummaryKey{channelId: channelID, model: l.Model, mode: l.Mode, fallback: l.Fallback}
}

type billingReportUpstreamTask struct {
	Model     string
	Mode      string
	Fallback  bool
	UserID    int
	TaskID    string
	Read      upstreamCacheEvidence
	Write     upstreamCacheEvidence
	Recovered *billingRecoveredSeconds
}

type billingReportUpstreamProjection struct {
	dayStart     int64
	ChannelID    int
	ChannelName  string
	Found        bool
	URLKey       string
	UpstreamName string
	Discount     *ProviderChannelBillingDiscount
	Leaves       []billingReportUpstreamLeaf
	Tasks        []billingReportUpstreamTask
	BuiltAt      int64
}

func (p billingReportUpstreamProjection) channel() Channel {
	channel := Channel{Id: p.ChannelID, Name: p.ChannelName}
	if p.Found && normalizeBillingURLGroupKey(p.URLKey) == p.URLKey {
		baseURL := p.URLKey
		channel.BaseURL = &baseURL
	}
	return channel
}

func (report *BillingReportManifest) upstreamRows(ctx context.Context, channelIDs []int) ([]billingReportUpstreamProjection, error) {
	query := DB.WithContext(ctx).Where("generation IN ?", report.ids())
	if len(channelIDs) > 0 {
		query = query.Where("channel_id IN ?", channelIDs)
	}
	var rows []billingReportUpstreamDay
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	days := make(map[string]int64, len(report.generations))
	for _, generation := range report.generations {
		days[generation.ID] = generation.Start
	}
	type payloadKey struct {
		generation string
		channelID  int
	}
	var chunkedChannels []int
	chunked := make(map[payloadKey]*strings.Builder)
	seenChannels := make(map[int]bool)
	for _, row := range rows {
		if row.Payload != "" {
			continue
		}
		chunked[payloadKey{row.Generation, row.ChannelID}] = new(strings.Builder)
		if !seenChannels[row.ChannelID] {
			seenChannels[row.ChannelID] = true
			chunkedChannels = append(chunkedChannels, row.ChannelID)
		}
	}
	for offset := 0; offset < len(chunkedChannels); offset += 100 {
		query := DB.WithContext(ctx).Where("generation IN ? AND kind = ? AND scope_id IN ?",
			report.ids(), reportBillingUpstreamPayload, chunkedChannels[offset:min(offset+100, len(chunkedChannels))])
		var last dailyReportPayloadPart
		for {
			batch := query.Session(&gorm.Session{}).Order("generation, scope_id, part").Limit(32)
			if last.Generation != "" {
				batch = batch.Where("(generation > ? OR (generation = ? AND (scope_id > ? OR (scope_id = ? AND part > ?))))",
					last.Generation, last.Generation, last.ScopeID, last.ScopeID, last.Part)
			}
			var parts []dailyReportPayloadPart
			if err := batch.Find(&parts).Error; err != nil {
				return nil, err
			}
			for _, part := range parts {
				if out := chunked[payloadKey{part.Generation, part.ScopeID}]; out != nil {
					out.WriteString(string(part.Payload))
				}
			}
			if len(parts) < 32 {
				break
			}
			last = parts[len(parts)-1]
		}
	}
	result := make([]billingReportUpstreamProjection, 0, len(rows))
	for _, row := range rows {
		var p billingReportUpstreamProjection
		raw := string(row.Payload)
		if out := chunked[payloadKey{row.Generation, row.ChannelID}]; out != nil {
			raw = out.String()
		}
		if err := common.UnmarshalJsonStr(raw, &p); err != nil {
			return nil, err
		}
		p.dayStart = days[row.Generation]
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BuiltAt < result[j].BuiltAt })
	return result, nil
}

func mergeBillingReportUpstream(rows []billingReportUpstreamProjection, discounts map[int]ProviderChannelBillingDiscount) (map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, error) {
	items := make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary)
	taskCache := newUpstreamTaskCacheTracker()
	recovered := make(map[upstreamRecoveredSecondsKey]billingRecoveredSeconds)
	for _, p := range rows {
		for _, leaf := range p.Leaves {
			key := leaf.key(p.ChannelID)
			item := items[key]
			if item == nil {
				item = &ProviderBillingPlatformSummary{ChannelId: p.ChannelID, ChannelName: p.ChannelName, ProviderModel: leaf.Model,
					ProviderModelFallback: leaf.Fallback, BillingMode: leaf.Mode, originalComplete: true, customerModels: make(map[string]struct{})}
				items[key] = item
			}
			accumulateProviderBillingUsage(&item.Usage, leaf.Usage)
			quality := leaf.Quality
			legacyAmount := (quality != nil && quality.HasEstimatedOriginal) ||
				(!leaf.RecordedOriginalBasis && leaf.SettlementRows > 0 && leaf.Known)
			if legacyAmount {
				leaf.Original, leaf.Reference, leaf.Basis = decimal.Zero, decimal.Zero, nil
				leaf.Known, leaf.Complete = false, false
				leaf.Reasons = mergeBillingEstimateReasons(leaf.Reasons, billingOriginalQuotaNotRecorded)
				if quality != nil {
					copy := *quality
					quality = &copy
				}
				ensureBillingReconciliationQuality(&quality).HasEstimatedOriginal = false
				quality.MissingHistoricalPriceRows += int64(leaf.SettlementRows)
			}
			accumulateBillingReconciliationQuality(&item.DataQuality, quality)
			item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, leaf.Reasons...)
			item.originalQuota = item.originalQuota.Add(leaf.Original)
			item.originalQuotaKnown = item.originalQuotaKnown || leaf.Known
			item.originalComplete = item.originalComplete && leaf.Complete
			item.testMoneyRows += leaf.TestRows
			item.moneyRows += leaf.MoneyRows
			item.settlementRows += leaf.SettlementRows
			for name := range leaf.Models {
				item.customerModels[name] = struct{}{}
			}
			if discount, ok := discounts[p.ChannelID]; ok {
				coefficient := discount.Changes.valueAt(discount.Discount, p.dayStart)
				reference := decimal.Zero
				if len(leaf.Basis) == 0 && !leaf.Original.IsZero() {
					if p.Discount == nil || !p.Discount.Changes.valueAt(p.Discount.Discount, p.dayStart).Equal(coefficient) {
						return nil, errors.New("published upstream amount is missing its per-record basis")
					}
					reference = leaf.Reference
				}
				for value, count := range leaf.Basis {
					basis, err := decimal.NewFromString(value)
					if err != nil {
						return nil, err
					}
					reference = reference.Add(UpstreamReferenceQuota(basis, coefficient).Mul(decimal.NewFromInt(count)))
				}
				item.referenceQuota = item.referenceQuota.Add(reference)
			}
		}
		for _, task := range p.Tasks {
			item := providerBillingSummaryKey{channelId: p.ChannelID, model: task.Model, mode: task.Mode, fallback: task.Fallback}
			if task.Recovered != nil {
				key := upstreamRecoveredSecondsKey{upstreamTaskSecondsKey{task.UserID, p.ChannelID, task.TaskID}, item}
				if prev, found := recovered[key]; found {
					excluded := task.Recovered.Value
					if task.Recovered.SourceID < prev.SourceID {
						recovered[key], excluded = *task.Recovered, prev.Value
					}
					value := items[item].Usage.Seconds.Sub(excluded)
					items[item].Usage.Seconds = &value
				} else {
					recovered[key] = *task.Recovered
				}
				continue
			}
			key := billingTaskCacheKey{userID: task.UserID, taskID: task.TaskID, item: item}
			bucket := taskCache.buckets[key]
			if bucket == nil {
				taskCache.buckets[key] = &upstreamTaskCacheBucket{item: item, read: task.Read, write: task.Write}
			} else {
				bucket.read = mergeUpstreamCacheEvidence(bucket.read, task.Read)
				bucket.write = mergeUpstreamCacheEvidence(bucket.write, task.Write)
			}
		}
	}
	taskCache.flushInto(items)
	return items, nil
}

func (report *BillingReportManifest) upstreamSummary(ctx context.Context, start, end int64, urlKey string, channelID int) (ProviderURLSummary, error) {
	var scope []int
	if channelID > 0 {
		scope = []int{channelID}
	}
	rows, err := report.upstreamRows(ctx, scope)
	if err != nil {
		return ProviderURLSummary{}, err
	}
	discounts, err := currentReportUpstreamDiscounts(ctx, start, rows)
	if err != nil {
		return ProviderURLSummary{}, err
	}
	return summarizeReportUpstream(rows, discounts, start, end, urlKey, true)
}

// Published generations fix usage, row amounts and URL membership. The
// channel-month coefficient is a separate, editable input to reference amounts.
func currentReportUpstreamDiscounts(ctx context.Context, start int64, rows []billingReportUpstreamProjection) (map[int]ProviderChannelBillingDiscount, error) {
	var ids []int
	seen := make(map[int]struct{})
	for _, row := range rows {
		if _, ok := seen[row.ChannelID]; !ok {
			seen[row.ChannelID] = struct{}{}
			ids = append(ids, row.ChannelID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	date := time.Unix(start, 0).In(UsageAnalyticsZone())
	month := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location()).Unix()
	records, err := loadProviderChannelBillingDiscounts(ctx, month, ids)
	if err != nil {
		return nil, err
	}
	for id, record := range records {
		if record.PendingReason != "" {
			delete(records, id)
		}
	}
	return records, nil
}

func summarizeReportUpstream(rows []billingReportUpstreamProjection, discounts map[int]ProviderChannelBillingDiscount, start, end int64, urlKey string, includeModels bool) (ProviderURLSummary, error) {
	result := ProviderURLSummary{Groups: []ProviderURLGroupSummary{}}
	partitions := make(map[string][]billingReportUpstreamProjection)
	for _, row := range rows {
		if urlKey == "" || row.URLKey == urlKey {
			partitions[row.URLKey] = append(partitions[row.URLKey], row)
		}
	}
	for _, days := range partitions {
		items, err := mergeBillingReportUpstream(days, discounts)
		if err != nil {
			return result, err
		}
		channels := make(map[int]Channel)
		names := make(map[string]string)
		upstreamNames := make(map[int]string)
		for _, p := range days {
			upstreamNames[p.ChannelID] = p.UpstreamName
			if p.Found {
				channels[p.ChannelID] = p.channel()
			}
			names[p.URLKey] = p.UpstreamName
		}
		for key, item := range items {
			item.DetailFilter = BillingReconciliationDetailFilter{StartTimestamp: start, EndTimestamp: end, ChannelId: key.channelId, BillingMode: key.mode}
			if len(item.customerModels) == 1 {
				for name := range item.customerModels {
					item.DetailFilter.ModelName = name
				}
			}
		}
		grouped, err := assembleProviderBillingURLSummary(items, channels, discounts, names, urlKey, includeModels)
		if err != nil {
			return result, err
		}
		for i := range grouped.Groups {
			for j := range grouped.Groups[i].Channels {
				channel := &grouped.Groups[i].Channels[j]
				channel.UpstreamName = upstreamNames[channel.ChannelId]
			}
		}
		result.Groups = append(result.Groups, grouped.Groups...)
		accumulateBillingReconciliationQuality(&result.DataQuality, grouped.DataQuality)
	}
	sort.Slice(result.Groups, func(i, j int) bool { return providerURLGroupLess(result.Groups[i], result.Groups[j]) })
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return result, nil
}

func (report *BillingReportManifest) freezeUpstreamExportScope(ctx context.Context, start int64, role int, channelIDs []int, urlKey string, allChannels bool) (*UpstreamExportScope, error) {
	rows, err := report.upstreamRows(ctx, channelIDs)
	if err != nil {
		return nil, err
	}
	scope := &UpstreamExportScope{AllChannels: allChannels, URLKey: urlKey, ChannelIds: channelIDs,
		Channels: make(map[int]UpstreamExportChannel), IncludeRootFields: role >= common.RoleRootUser}
	discounts, err := currentReportUpstreamDiscounts(ctx, start, rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if urlKey != "" && row.URLKey != urlKey {
			continue
		}
		scope.Channels[row.ChannelID] = UpstreamExportChannel{Name: row.ChannelName, UpstreamName: row.UpstreamName}
		if len(channelIDs) == 1 && urlKey == "" {
			scope.GroupName = row.UpstreamName
		}
	}
	for id, channel := range scope.Channels {
		if d, ok := discounts[id]; ok {
			p := providerChannelDiscountProjection(d)
			channel.Discount = &p
			scope.Channels[id] = channel
		}
	}
	for _, id := range channelIDs {
		if _, ok := scope.Channels[id]; !ok && report.PublishedDays > 0 {
			return nil, ErrCustomerExportNotFound
		}
	}
	if urlKey != "" {
		var display ProviderURLGroupDisplayName
		if err := DB.WithContext(ctx).Where("url_hash = ?", providerURLGroupHash(urlKey)).Limit(1).Find(&display).Error; err != nil {
			return nil, err
		}
		scope.GroupName = display.Name
	}
	return scope, nil
}
