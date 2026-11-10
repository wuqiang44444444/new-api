package model

import "github.com/QuantumNous/new-api/common"

// ValidateRelayQuota enforces the quota policy for an authenticated token.
func (token *Token) ValidateRelayQuota() error {
	if token.Status != common.TokenStatusEnabled {
		return ErrTokenInvalid
	}
	if token.UnlimitedQuota || token.RemainQuota > 0 {
		return nil
	}
	if !common.RedisEnabled {
		token.Status = common.TokenStatusExhausted
		if err := token.SelectUpdate(); err != nil {
			common.SysLog(common.LogText("failed to update token status%s", err.Error()))
		}
	}
	return ErrTokenInvalid
}
