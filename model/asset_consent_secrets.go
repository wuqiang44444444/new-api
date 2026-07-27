package model

import "gorm.io/gorm"

func ClearRealPersonVerificationSecretsTx(tx *gorm.DB, authorizationID int64) error {
	if tx == nil || authorizationID == 0 {
		return nil
	}
	return tx.Model(&RealPersonVerificationSession{}).
		Where("authorization_id = ?", authorizationID).
		Updates(map[string]any{
			"verification_handle_ciphertext": "",
			"h5_url_ciphertext":              "",
			"verification_token_hash":        nil,
		}).Error
}
