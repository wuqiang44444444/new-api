package model

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const billingReportDaySeconds int64 = 86400

type billingReportHead struct {
	Start      int64  `gorm:"primaryKey;autoIncrement:false"`
	Generation string `gorm:"size:32;not null"`
	AsOf       int64
}

type billingReportGeneration struct {
	ID         string `gorm:"size:32;primaryKey"`
	Start      int64
	AsOf       int64
	UpperLogID int64
	Rows       int64
	Incomplete bool
	Published  bool
	RetiredAt  int64 `gorm:"index"`
	CreatedAt  int64
}

type billingReportCustomerDay struct {
	Generation string `gorm:"size:32;primaryKey"`
	UserID     int    `gorm:"primaryKey;autoIncrement:false;index"`
	Summary    BillingStatementJSON
	Leaves     BillingStatementJSON
}

type billingReportUpstreamDay struct {
	Generation string `gorm:"size:32;primaryKey"`
	ChannelID  int    `gorm:"primaryKey;autoIncrement:false"`
	Payload    BillingStatementJSON
}

type billingReportLine struct {
	Generation        string `gorm:"size:32;primaryKey;index:idx_brl_customer,priority:1;index:idx_brl_channel,priority:1;index:idx_brl_fund,priority:1"`
	SourceID          int64  `gorm:"primaryKey;autoIncrement:false;index:idx_brl_customer,priority:3;index:idx_brl_channel,priority:3;index:idx_brl_fund,priority:4"`
	UserID            int    `gorm:"index:idx_brl_customer,priority:2;index:idx_brl_fund,priority:2"`
	ChannelID         int    `gorm:"index:idx_brl_channel,priority:2"`
	TokenID           int
	LogType           int
	CreatedAt         int64
	RequestID         string `gorm:"size:64"`
	UpstreamRequestID string `gorm:"size:128"`
	CustomerModel     string `gorm:"size:255"`
	ProviderModel     string `gorm:"size:255"`
	ModelFallback     bool
	BillingMode       string `gorm:"size:32"`
	TokenName         string
	GroupName         string
	FundGroup         string `gorm:"size:75;index:idx_brl_fund,priority:3"`
	Customer          BillingStatementJSON
	Upstream          BillingStatementJSON
	OriginalExact     string
	URLKey            string
	Username          string
	RootTaskRef       string
	CustomerCharge    int64
	BillableRequests  int64
	InputTokens       int64
	OutputTokens      int64
	EvidenceTags      string
}

type BillingReportManifest struct {
	Version        string `json:"version"`
	PublishedDays  int    `json:"published_days"`
	IncompleteDays int    `json:"incomplete_days,omitempty"`
	ExpectedDays   int    `json:"expected_days"`
	AsOf           int64  `json:"as_of"`
	EstimatedAt    int64  `json:"estimated_at,omitempty"`
	Estimated      bool   `json:"estimated,omitempty"`
	generations    []billingReportGeneration
}

type billingReportContextKey struct{}

func BillingReportFromContext(ctx context.Context) *BillingReportManifest {
	report, _ := ctx.Value(billingReportContextKey{}).(*BillingReportManifest)
	return report
}

// ResolveBillingReport pins the date map once. A requested generation must have
// been published for a day within this period. Empty coverage has its own token.
func ResolveBillingReport(ctx context.Context, start, end int64, version string) (context.Context, *BillingReportManifest, error) {
	if start <= 0 || end < start || end-start > 31*billingReportDaySeconds {
		return ctx, nil, common.NewMessage("Invalid time range")
	}

	now := common.GetTimestamp()
	today := time.Unix(now, 0).In(UsageAnalyticsZone())
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location()).Unix()
	report := &BillingReportManifest{Version: "-"}
	for day := start; day <= min(end, todayStart); day += billingReportDaySeconds {
		report.ExpectedDays++
	}
	var rows []billingReportGeneration
	switch {
	case version == "-":
	case version != "":
		ids := strings.Split(version, ",")
		if len(ids) > 31 {
			return ctx, nil, common.NewMessage("Invalid billing report version")
		}
		for _, id := range ids {
			if len(id) != 32 {
				return ctx, nil, common.NewMessage("Invalid billing report version")
			}
		}
		if err := DB.WithContext(ctx).Where("id IN ? AND published = ? AND start >= ? AND start <= ?", ids, true, start, end).
			Order("start ASC").Find(&rows).Error; err != nil {
			return ctx, nil, err
		}
		if len(rows) != len(ids) {
			return ctx, nil, common.NewMessage("Billing report version is unavailable")
		}
	default:
		if err := DB.WithContext(ctx).Table("billing_report_generations AS generation").Select("generation.*").
			Joins("JOIN billing_report_heads AS head ON head.generation = generation.id").
			Where("head.start >= ? AND head.start <= ?", start, min(end, todayStart)).
			Where("(head.start = ? OR head.as_of >= head.start + ?)", todayStart, billingReportDaySeconds+3*3600).
			Order("head.start ASC").Find(&rows).Error; err != nil {
			return ctx, nil, err
		}
	}
	var issues map[int64]bool
	if len(rows) > 0 {
		var err error
		issues, err = dailyReportCoverageIssues(ctx, DailyReportBilling, start, end+1)
		if err != nil {
			return ctx, nil, err
		}
	}
	ids := make([]string, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	for _, row := range rows {
		if seen[row.Start] {
			return ctx, nil, common.NewMessage("Invalid billing report version")
		}
		seen[row.Start] = true
		ids = append(ids, row.ID)
		if report.AsOf == 0 || row.AsOf < report.AsOf {
			report.AsOf = row.AsOf
		}
		if row.Incomplete || issues[row.Start] {
			report.IncompleteDays++
		}
		if row.AsOf < row.Start+billingReportDaySeconds+3*3600 {
			report.Estimated = true
			report.EstimatedAt = max(report.EstimatedAt, row.AsOf)
		}
	}
	if len(ids) > 0 {
		report.Version = strings.Join(ids, ",")
	}
	report.PublishedDays = len(ids)
	report.generations = rows
	return context.WithValue(ctx, billingReportContextKey{}, report), report, nil
}

func (report *BillingReportManifest) ids() []string {
	ids := make([]string, 0, len(report.generations))
	for _, row := range report.generations {
		ids = append(ids, row.ID)
	}
	return ids
}

func (report *BillingReportManifest) lines(ctx context.Context) *gorm.DB {
	return DB.WithContext(ctx).Model(&billingReportLine{}).Where("generation IN ?", report.ids())
}
