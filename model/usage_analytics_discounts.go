package model

// 日周用量沿用月度账单的冻结折扣解析与金额还原。这里只组合已被用量管道
// 接纳的结算事实，不按当前配置查价，也不把结算日志条数作为调用次数。
func (a *usageMetricsAcc) observeCustomerDiscount(row usageLogRow) {
	if a.discounts == nil {
		a.discounts = newBillingDiscountCombinationAccumulator()
	}
	fact := row.fact
	fact.ModelName = ""
	a.discounts.observe(fact, 0, row.parsed)
}

// mergeUsageDiscounts 保留未舍入金额。跨日/任务合并超出组合上限时，整组收敛
// 为明确的“其他组合”，避免 map 遍历顺序决定哪些折扣被保留。
func (a *billingDiscountCombinationAccumulator) mergeUsageDiscounts(source *billingDiscountCombinationAccumulator) {
	if source == nil {
		return
	}
	count := len(a.combinations)
	for key := range source.combinations {
		if a.combinations[key] == nil {
			count++
		}
	}
	if count > billingDiscountCombinationCap || a.overflow != nil || source.overflow != nil {
		if a.overflow == nil {
			a.overflow = &billingDiscountCombinationEntry{combo: BillingDiscountCombination{
				Other: true, ContractApplicable: combinationContractApplicableUnknown,
				EstimateReasons: []string{BillingEstimateCombinationLimit},
			}}
		}
		for _, entry := range a.combinations {
			accumulateBillingReconciliationUsage(&a.overflow.combo.Usage, entry.combo.Usage)
		}
		clear(a.combinations)
		for _, entry := range source.combinations {
			accumulateBillingReconciliationUsage(&a.overflow.combo.Usage, entry.combo.Usage)
		}
		if source.overflow != nil {
			accumulateBillingReconciliationUsage(&a.overflow.combo.Usage, source.overflow.combo.Usage)
		}
		return
	}
	for key, sourceEntry := range source.combinations {
		entry := a.combinations[key]
		if entry == nil {
			copy := *sourceEntry
			a.combinations[key] = &copy
			continue
		}
		accumulateBillingReconciliationUsage(&entry.combo.Usage, sourceEntry.combo.Usage)
		entry.originalQuota = entry.originalQuota.Add(sourceEntry.originalQuota)
		entry.originalComplete = entry.originalComplete && sourceEntry.originalComplete
		entry.combo.InputTokensKnown = entry.combo.InputTokensKnown && sourceEntry.combo.InputTokensKnown
		entry.combo.EstimateReasons = mergeBillingEstimateReasons(entry.combo.EstimateReasons, sourceEntry.combo.EstimateReasons...)
	}
}

func (agg *usageAggregation) customerModelDiscounts(token int, row *UsageCustomerModelRow) {
	// 结算未完成或日志交付不完整时，不能把部分预扣解释成最终折扣构成。
	if row.Total.RowsMissingMoney > 0 || row.Total.RowsMoneyPending > 0 {
		return
	}
	combined := newBillingDiscountCombinationAccumulator()
	for day := range agg.period.Days {
		if acc := agg.customer[usageCustomerKey{day: day, token: token, model: row.ModelName}]; acc != nil {
			combined.mergeUsageDiscounts(acc.discounts)
		}
	}
	row.DiscountCombinations = combined.finalize()
	for i := range row.DiscountCombinations {
		row.DiscountCombinations[i].GroupId = int64(token)
		row.DiscountCombinations[i].ModelName = row.ModelName
	}
}
