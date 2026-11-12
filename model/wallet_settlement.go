package model

import (
	"github.com/QuantumNous/new-api/common"
)

// SettleWalletQuota commits the adjustment before it can become a billing fact.
func SettleWalletQuota(userID, chargeDelta int) error {
	if chargeDelta == 0 {
		return nil
	}
	if chargeDelta < 0 {
		return IncreaseUserQuota(userID, -chargeDelta, true)
	}
	if !common.RedisEnabled {
		return persistUserQuotaDelta(userID, -chargeDelta)
	}
	cacheResult, cacheErr := cacheApplyUserQuotaDelta(userID, -int64(chargeDelta))
	if cacheErr != nil {
		common.SysLog(common.LogText("wallet cache settlement unavailable: %s", cacheErr.Error()))
	}
	err := persistUserQuotaDelta(userID, -chargeDelta)
	if err != nil && cacheErr == nil && cacheResult == cacheQuotaOK {
		compensated, compensateErr := cacheApplyUserQuotaDelta(userID, int64(chargeDelta))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(common.LogText("failed to compensate wallet cache settlement: result=%d error=%v", compensated, compensateErr))
		}
	}
	return err
}
