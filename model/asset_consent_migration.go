package model

// migrateEmptyAssetReceiptHashes keeps pending authorizations nullable so the
// unique receipt index only applies after consent has produced a receipt.
func migrateEmptyAssetReceiptHashes() error {
	return DB.Model(&RealPersonAuthorization{}).
		Where("receipt_token_hash = ?", "").
		Update("receipt_token_hash", nil).Error
}
