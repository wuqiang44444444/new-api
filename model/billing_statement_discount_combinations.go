package model

import (
	"sort"

	"github.com/shopspring/decimal"
)

// Discount combination projection (plan 1.3 / section 4): a second read-only
// projection over the same settled logs as the monthly statement. Rows are
// keyed by frozen discount facts only -- effective group ratio, contract
// applicability, contract identity/version (when known) and ratio values.
// Rows are never merged by current names; legacy rows whose contract identity
// is missing but ratio matches aggregate into an "identity unrecorded"
// combination. When any row's original price cannot be reconstructed, the
// combination reports original/discount as unavailable; net is preserved.

// billingDiscountCombinationCap bounds combination keys; keys beyond the cap
// merge into the explicit overflow row so totals stay reconcilable.
const billingDiscountCombinationCap = 300

const (
	combinationContractApplicableYes     = "yes"
	combinationContractApplicableNo      = "no"
	combinationContractApplicableUnknown = "unknown"
	combinationContractUnrecorded        = "unrecorded"
)

// BillingDiscountCombination is one customer-visible discount combination row
// of the monthly statement (plan section 4).
type BillingDiscountCombination struct {
	EstimateReasons    []string                   `json:"estimate_reasons,omitempty"`
	InputTokensKnown   bool                       `json:"input_tokens_known"`
	GroupId            int64                      `json:"group_id"`
	ModelName          string                     `json:"model_name"`
	BillingMode        string                     `json:"billing_mode"`
	GroupName          string                     `json:"group_name"`
	GroupRatioSource   string                     `json:"group_ratio_source"`
	GroupRatio         *float64                   `json:"group_ratio,omitempty"`
	ContractApplicable string                     `json:"contract_applicable"`
	ContractName       string                     `json:"contract_name,omitempty"`
	ContractIdKnown    bool                       `json:"contract_id_known,omitempty"`
	ContractId         int64                      `json:"contract_id,omitempty"`
	ContractVersion    int64                      `json:"contract_version,omitempty"`
	ContractRatio      *float64                   `json:"contract_ratio,omitempty"`
	Usage              BillingReconciliationUsage `json:"usage"`
	OriginalKnown      bool                       `json:"original_known"`
	OriginalQuota      *int64                     `json:"original_quota,omitempty"`
	DiscountQuota      *int64                     `json:"discount_quota,omitempty"`
	Other              bool                       `json:"other,omitempty"`
}

// billingDiscountCombinationKey is composed of frozen-at-settlement facts
// only. Contract identity participates in the key only when the discount was
// explicitly applied and the identity is known; rows whose identity is missing
// aggregate by ratio into an "identity unrecorded" combination.
type billingDiscountCombinationKey struct {
	groupId            int64
	model              string
	mode               string
	groupName          string
	groupRatioSource   string
	groupRatio         float64
	hasGroupRatio      bool
	contractApplicable string
	contractId         int64
	contractIdKnown    bool
	contractVersion    int64
	contractRatio      float64
	hasContractRatio   bool
}

type billingDiscountCombinationEntry struct {
	combo            BillingDiscountCombination
	originalQuota    decimal.Decimal
	originalComplete bool
}

type billingDiscountCombinationAccumulator struct {
	combinations map[billingDiscountCombinationKey]*billingDiscountCombinationEntry
	overflow     *billingDiscountCombinationEntry
}

func newBillingDiscountCombinationAccumulator() *billingDiscountCombinationAccumulator {
	return &billingDiscountCombinationAccumulator{
		combinations: make(map[billingDiscountCombinationKey]*billingDiscountCombinationEntry),
	}
}

func billingContractApplicableState(parsed parsedBillingReconciliationLog) string {
	if parsed.contractApplicableKnown && !parsed.contractApplicable && parsed.contractDiscountRatio != nil {
		return combinationContractApplicableUnknown
	}
	if parsed.contractDiscountRatio != nil && *parsed.contractDiscountRatio > 0 {
		return combinationContractApplicableYes
	}
	if parsed.contractApplicableKnown && !parsed.contractApplicable {
		return combinationContractApplicableNo
	}
	if parsed.contractEvidence || parsed.contractApplicableKnown || parsed.contractId > 0 || parsed.contractVersion > 0 || parsed.contractName != "" {
		return combinationContractApplicableUnknown
	}
	return combinationContractUnrecorded
}

// observe accumulates one settled log row. Money follows exactly the same
// pre-discount reconstruction as accumulateBillingReconciliationPrice:
// quota / G / C, refunds negative, never rounded per row. A zero-quota row is
// an exact zero money fact and does not invalidate the combination.
func (a *billingDiscountCombinationAccumulator) observe(log billingReconciliationLog, selectedGroupId int64, parsed parsedBillingReconciliationLog) {
	if log.Type != LogTypeConsume && log.Type != LogTypeRefund {
		return
	}
	key := billingDiscountCombinationKey{
		groupId:            selectedGroupId,
		groupName:          parsed.groupName,
		groupRatioSource:   parsed.groupRatioSource,
		model:              log.ModelName,
		mode:               parsed.billingMode,
		contractApplicable: billingContractApplicableState(parsed),
	}
	if parsed.discountRatio != nil && *parsed.discountRatio > 0 {
		key.hasGroupRatio = true
		key.groupRatio = *parsed.discountRatio
	}
	if key.contractApplicable == combinationContractApplicableYes {
		if parsed.contractDiscountRatio != nil && *parsed.contractDiscountRatio > 0 {
			key.hasContractRatio = true
			key.contractRatio = *parsed.contractDiscountRatio
		}
		if parsed.contractId > 0 {
			key.contractIdKnown = true
			key.contractId = int64(parsed.contractId)
			key.contractVersion = int64(parsed.contractVersion)
		}
	}
	entry := a.combinations[key]
	if entry == nil {
		if len(a.combinations) >= billingDiscountCombinationCap {
			a.observeOverflow(log, parsed)
			return
		}
		entry = &billingDiscountCombinationEntry{
			combo: BillingDiscountCombination{
				InputTokensKnown:   true,
				GroupId:            selectedGroupId,
				GroupName:          key.groupName,
				GroupRatioSource:   key.groupRatioSource,
				ModelName:          log.ModelName,
				BillingMode:        parsed.billingMode,
				GroupRatio:         ratioPointerForCombination(key.hasGroupRatio, key.groupRatio),
				ContractApplicable: key.contractApplicable,
				ContractName:       parsed.contractName,
				ContractIdKnown:    key.contractIdKnown,
				ContractId:         key.contractId,
				ContractVersion:    key.contractVersion,
				ContractRatio:      ratioPointerForCombination(key.hasContractRatio, key.contractRatio),
				OriginalKnown:      true,
			},
			originalComplete: true,
		}
		a.combinations[key] = entry
	}
	if parsed.inputTokensUnavailable {
		entry.combo.InputTokensKnown = false
	}
	accumulateBillingReconciliationLog(&entry.combo.Usage, log, parsed)
	quota := max(int64(log.Quota), int64(0))
	if quota == 0 {
		return
	}
	// 还原口径与 accumulateBillingReconciliationPrice 完全一致：组与合同事实齐备且
	// 无附加费才还原；完全没有合同记录时按 C=1，残缺合同仍不可估算。
	if reasons := billingStatementEstimateReasons(parsed); len(reasons) > 0 {
		entry.combo.EstimateReasons = mergeBillingEstimateReasons(entry.combo.EstimateReasons, reasons...)
		entry.originalComplete = false
		return
	}
	original := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(key.groupRatio))
	if key.hasContractRatio {
		original = original.Div(decimal.NewFromFloat(key.contractRatio))
	}
	if log.Type == LogTypeRefund {
		original = original.Neg()
	}
	entry.originalQuota = entry.originalQuota.Add(original)
}

func ratioSortKey(ratio *float64) float64 {
	if ratio == nil {
		return -1
	}
	return *ratio
}

func ratioPointerForCombination(known bool, value float64) *float64 {
	if !known || value <= 0 {
		return nil
	}
	return &value
}

// observeOverflow merges a row into the explicit overflow combination once the
// key cap is reached. Facts are not shown for this row; money and usage still
// count exactly once toward the totals.
func (a *billingDiscountCombinationAccumulator) observeOverflow(log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	if a.overflow == nil {
		a.overflow = &billingDiscountCombinationEntry{
			combo: BillingDiscountCombination{
				ContractApplicable: combinationContractApplicableUnknown,
				OriginalKnown:      false,
				Other:              true,
				EstimateReasons:    []string{BillingEstimateCombinationLimit},
			},
			originalComplete: false,
		}
	}
	accumulateBillingReconciliationLog(&a.overflow.combo.Usage, log, parsed)
}

// finalize returns the customer-visible combination rows. Original prices use
// the same signed decimal accumulation and rounding as the statement level
// (billingStatementOriginalQuota), refunds keep their sign, and rows sort by
// descending absolute net amount so the page shows the dominant combinations
// first.
func (a *billingDiscountCombinationAccumulator) finalize() []BillingDiscountCombination {
	list := make([]BillingDiscountCombination, 0, len(a.combinations)+1)
	for _, entry := range a.combinations {
		list = append(list, finalizeDiscountCombinationEntry(entry))
	}
	if a.overflow != nil {
		list = append(list, finalizeDiscountCombinationEntry(a.overflow))
	}
	// 排序必须完全由输入决定（组合来自 map 遍历，随机初序）：
	// |净额| 降序 → 模型 → 计费方式 → 分组倍率 → 合同状态 → 合同倍率 → 身份。
	sort.Slice(list, func(i, j int) bool {
		left, right := &list[i], &list[j]
		absLeft, absRight := left.Usage.NetQuota, right.Usage.NetQuota
		if absLeft < 0 {
			absLeft = -absLeft
		}
		if absRight < 0 {
			absRight = -absRight
		}
		if absLeft != absRight {
			return absLeft > absRight
		}
		if left.ModelName != right.ModelName {
			return left.ModelName < right.ModelName
		}
		if left.BillingMode != right.BillingMode {
			return left.BillingMode < right.BillingMode
		}
		if leftOther, rightOther := left.Other, right.Other; leftOther != rightOther {
			return !leftOther
		}
		if leftRatio, rightRatio := ratioSortKey(left.GroupRatio), ratioSortKey(right.GroupRatio); leftRatio != rightRatio {
			return leftRatio < rightRatio
		}
		if left.ContractApplicable != right.ContractApplicable {
			return left.ContractApplicable < right.ContractApplicable
		}
		if l, r := ratioSortKey(left.ContractRatio), ratioSortKey(right.ContractRatio); l != r {
			return l < r
		}
		if left.GroupName != right.GroupName {
			return left.GroupName < right.GroupName
		}
		if left.GroupRatioSource != right.GroupRatioSource {
			return left.GroupRatioSource < right.GroupRatioSource
		}
		if left.GroupId != right.GroupId {
			return left.GroupId < right.GroupId
		}
		if left.ContractIdKnown != right.ContractIdKnown {
			return !left.ContractIdKnown
		}
		if left.ContractId != right.ContractId {
			return left.ContractId < right.ContractId
		}
		return left.ContractVersion < right.ContractVersion
	})
	return list
}

func finalizeDiscountCombinationEntry(entry *billingDiscountCombinationEntry) BillingDiscountCombination {
	combo := entry.combo
	finalizeBillingReconciliationUsage(&combo.Usage)
	if entry.originalComplete {
		combo.OriginalKnown = entry.originalComplete
		combo.OriginalQuota = billingStatementOriginalQuota(entry.originalQuota)
		combo.OriginalKnown = combo.OriginalQuota != nil
		if !combo.OriginalKnown {
			combo.EstimateReasons = mergeBillingEstimateReasons(combo.EstimateReasons, BillingEstimateAmountOutOfRange)
		}
		if combo.OriginalQuota != nil {
			discount := *combo.OriginalQuota - combo.Usage.NetQuota
			combo.DiscountQuota = &discount
		}
	} else {
		combo.OriginalKnown = false
	}
	return combo
}
