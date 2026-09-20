package model

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Legacy automatic reasons written by the retired model-level materializer.
// They are evidence markers, never proof on their own: a matching reason only
// counts when the referenced ancestor evidence also checks out.
const (
	legacyProviderDiscountReasonDefault = "automatic monthly default"
	legacyProviderDiscountReasonCopy    = "automatic copy from previous billing period"
)

type legacyProviderDiscountSnapshot struct {
	Discount         decimal.Decimal `json:"discount"`
	CopiedFromPeriod int64           `json:"copied_from_period"`
	Reason           string          `json:"reason"`
}

// Provenance kinds for legacy model-level discount records.
//
// manual  — a human wrote or last corrected this value.
// copy    — the record provably copied an ancestor value that was
//
//	human-confirmed at copy time (manual or another proven copy).
//
// default — the record was the system-generated coefficient 1.
// unknown — evidence is missing or contradictory; never converged.
const (
	providerDiscountProvenanceManual  = "manual"
	providerDiscountProvenanceCopy    = "copy"
	providerDiscountProvenanceDefault = "default"
	providerDiscountProvenanceUnknown = "unknown"
	migrateProvenanceDepthLimit       = 120
)

type legacyProviderDiscountEvidence struct {
	records map[string]ProviderBillingDiscount
	audits  map[string][]ProviderBillingAudit
	kinds   map[string]string
}

// migrateProviderModelDiscountsToChannel converges the retired model-level
// monthly discounts into channel-month records. It is a one-time, idempotent,
// non-destructive migration: legacy rows and their audit history are kept
// untouched, converged channel rows are created once, and every ambiguous
// channel-month is deliberately left pending manual fill. Conflicts never
// pick the first value, never average, and a system default never masquerades
// as a confirmed no-discount.
func migrateProviderModelDiscountsToChannel() error {
	var rows []ProviderBillingDiscount
	if err := DB.Order("period_start ASC, channel_id ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	var audits []ProviderBillingAudit
	if err := DB.Where("entity_type = ?", "discount").Order("id ASC").Find(&audits).Error; err != nil {
		return err
	}
	evidence := newLegacyProviderDiscountEvidence(rows, audits)

	type channelMonth struct {
		periodStart int64
		channelId   int
	}
	var groups []channelMonth
	groupsByKey := make(map[channelMonth][]string)
	for _, row := range rows {
		key := channelMonth{periodStart: row.PeriodStart, channelId: row.ChannelId}
		if _, ok := groupsByKey[key]; !ok {
			groups = append(groups, key)
		}
		groupsByKey[key] = append(groupsByKey[key], providerBillingEntityKey(row.PeriodStart, row.ChannelId, row.ProviderModel, row.BillingMode))
	}

	converged, pending, skipped := 0, 0, 0
	for _, group := range groups {
		exists, err := providerChannelDiscountExists(group.periodStart, group.channelId)
		if err != nil {
			return err
		}
		if exists {
			skipped++
			continue
		}
		recordKeys := groupsByKey[group]
		validCoefficients := make([]decimal.Decimal, 0, len(recordKeys))
		hasUnconfirmed := false
		for _, recordKey := range recordKeys {
			switch evidence.kindOf(recordKey) {
			case providerDiscountProvenanceManual, providerDiscountProvenanceCopy, providerDiscountProvenanceDefault:
				validCoefficients = append(validCoefficients, evidence.records[recordKey].Discount)
			default:
				hasUnconfirmed = true
			}
		}
		if len(validCoefficients) == 0 || hasUnconfirmed {
			if err := createMigratedProviderChannelDiscount(group.periodStart, group.channelId, decimal.Zero, len(recordKeys)); err != nil {
				return err
			}
			pending++
			continue
		}
		uniform := true
		for _, coefficient := range validCoefficients[1:] {
			if !coefficient.Equal(validCoefficients[0]) {
				uniform = false
				break
			}
		}
		if !uniform {
			if err := createMigratedProviderChannelDiscount(group.periodStart, group.channelId, decimal.Zero, len(recordKeys)); err != nil {
				return err
			}
			pending++
			continue
		}
		if err := createMigratedProviderChannelDiscount(group.periodStart, group.channelId, validCoefficients[0], len(recordKeys)); err != nil {
			return err
		}
		converged++
	}
	common.SysLog(fmt.Sprintf("provider channel discount migration: %d channel-months converged, %d pending manual fill, %d already present", converged, pending, skipped))
	return nil
}

func providerChannelDiscountExists(periodStart int64, channelId int) (bool, error) {
	var count int64
	if err := DB.Model(&ProviderChannelBillingDiscount{}).Where("period_start = ? AND channel_id = ?", periodStart, channelId).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func createMigratedProviderChannelDiscount(periodStart int64, channelId int, coefficient decimal.Decimal, sourceRecords int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		discount := ProviderChannelBillingDiscount{
			PeriodStart: periodStart, ChannelId: channelId,
			Discount: coefficient, Version: 1,
			Reason:    fmt.Sprintf("%s (%d record(s))", reasonChannelDiscountMigrated, sourceRecords),
			CreatedBy: 0, UpdatedBy: 0,
		}
		if coefficient.IsZero() {
			discount.PendingReason = "legacy_evidence_requires_confirmation"
			discount.Version = 0 // first manual confirmation uses expected_version=0
		}
		if err := tx.Create(&discount).Error; err != nil {
			return err
		}
		return createProviderBillingAudit(tx, providerChannelDiscountEntity, providerChannelDiscountEntityKey(periodStart, channelId), "create", nil, &discount, discount.Reason, 0)
	})
}

func newLegacyProviderDiscountEvidence(rows []ProviderBillingDiscount, audits []ProviderBillingAudit) *legacyProviderDiscountEvidence {
	evidence := &legacyProviderDiscountEvidence{
		records: make(map[string]ProviderBillingDiscount, len(rows)),
		audits:  make(map[string][]ProviderBillingAudit),
		kinds:   make(map[string]string, len(rows)),
	}
	for _, row := range rows {
		evidence.records[providerBillingEntityKey(row.PeriodStart, row.ChannelId, row.ProviderModel, row.BillingMode)] = row
	}
	for _, audit := range audits {
		key := audit.EntityKey
		evidence.audits[key] = append(evidence.audits[key], audit)
	}
	return evidence
}

// kindOf resolves the provenance of a record's current value.
func (e *legacyProviderDiscountEvidence) kindOf(recordKey string) string {
	if kind, ok := e.kinds[recordKey]; ok {
		return kind
	}
	kind, value := e.provenanceAt(recordKey, math.MaxInt64, math.MaxInt64, map[string]struct{}{recordKey: {}})
	if record, ok := e.records[recordKey]; !ok || !record.Discount.Equal(value) {
		kind = providerDiscountProvenanceUnknown
	}
	e.kinds[recordKey] = kind
	return kind
}

// provenanceAt resolves what a record's value and provenance were just before
// the given point in time (exclusive, ordered by CreatedAt then audit Id).
// Each inherited link is validated against the ancestor's value at copy time:
// later corrections of the source month never invalidate — or validate — the
// copy that already happened.
func (e *legacyProviderDiscountEvidence) provenanceAt(recordKey string, beforeTime int64, beforeId int64, visiting map[string]struct{}) (string, decimal.Decimal) {
	audits := e.audits[recordKey]
	var latest *ProviderBillingAudit
	for i := range audits {
		audit := &audits[i]
		if audit.CreatedAt > beforeTime || (audit.CreatedAt == beforeTime && audit.Id >= beforeId) {
			continue
		}
		if latest == nil || audit.CreatedAt > latest.CreatedAt || (audit.CreatedAt == latest.CreatedAt && audit.Id > latest.Id) {
			latest = audit
		}
	}
	if latest == nil {
		return providerDiscountProvenanceUnknown, decimal.Decimal{}
	}
	if latest.Action != "create" && latest.Action != "update" {
		return providerDiscountProvenanceUnknown, decimal.Zero
	}
	if latest.Action == "update" {
		// Only the human save path writes updates; the value after this audit
		// is operator-confirmed regardless of the typed reason.
		snapshot, ok := parseLegacyProviderDiscountSnapshot(latest.After)
		if !ok {
			return providerDiscountProvenanceUnknown, decimal.Decimal{}
		}
		return providerDiscountProvenanceManual, snapshot.Discount
	}
	snapshot, ok := parseLegacyProviderDiscountSnapshot(latest.After)
	if !ok {
		return providerDiscountProvenanceUnknown, decimal.Decimal{}
	}
	switch snapshot.Reason {
	case legacyProviderDiscountReasonDefault:
		if !snapshot.Discount.Equal(decimal.NewFromInt(1)) {
			return providerDiscountProvenanceUnknown, decimal.Zero
		}
		return providerDiscountProvenanceDefault, snapshot.Discount
	case legacyProviderDiscountReasonCopy:
		if snapshot.CopiedFromPeriod <= 0 || len(visiting) >= migrateProvenanceDepthLimit {
			return providerDiscountProvenanceUnknown, decimal.Decimal{}
		}
		record := e.records[recordKey]
		ancestorKey := providerBillingEntityKey(snapshot.CopiedFromPeriod, record.ChannelId, record.ProviderModel, record.BillingMode)
		if _, loop := visiting[ancestorKey]; loop {
			return providerDiscountProvenanceUnknown, decimal.Decimal{}
		}
		if _, exists := e.records[ancestorKey]; !exists {
			return providerDiscountProvenanceUnknown, decimal.Decimal{}
		}
		visiting[ancestorKey] = struct{}{}
		kind, value := e.provenanceAt(ancestorKey, latest.CreatedAt, latest.Id, visiting)
		delete(visiting, ancestorKey)
		switch kind {
		case providerDiscountProvenanceManual, providerDiscountProvenanceCopy, providerDiscountProvenanceDefault:
			if !value.Equal(snapshot.Discount) {
				return providerDiscountProvenanceUnknown, decimal.Decimal{}
			}
			return providerDiscountProvenanceCopy, snapshot.Discount
		default:
			return providerDiscountProvenanceUnknown, decimal.Decimal{}
		}
	case "":
		return providerDiscountProvenanceUnknown, decimal.Decimal{}
	default:
		return providerDiscountProvenanceManual, snapshot.Discount
	}
}

func parseLegacyProviderDiscountSnapshot(raw string) (legacyProviderDiscountSnapshot, bool) {
	var snapshot legacyProviderDiscountSnapshot
	if err := common.UnmarshalJsonStr(raw, &snapshot); err != nil {
		return snapshot, false
	}
	if snapshot.Discount.LessThanOrEqual(decimal.Zero) || snapshot.Discount.GreaterThan(decimal.NewFromInt(1)) || !snapshot.Discount.Equal(snapshot.Discount.Round(8)) {
		return snapshot, false
	}
	return snapshot, true
}
