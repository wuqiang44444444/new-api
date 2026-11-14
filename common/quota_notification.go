package common

// EffectiveQuotaWarningThreshold is shared by the notification and account API.
func EffectiveQuotaWarningThreshold(personal float64) int {
	if personal >= 1 && personal <= MaxWalletQuota {
		return int(personal)
	}
	return QuotaRemindThreshold
}
