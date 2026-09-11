package model

import "gorm.io/gorm"

// customerSettlementLogs excludes the exact records written by native channel
// tests: they record a calculated cost without debiting the customer's wallet.
// Do not exclude token_id=0 alone; playground calls use it for real settlements.
// Provider usage queries deliberately do not apply this scope.
func customerSettlementLogs(db *gorm.DB) *gorm.DB {
	return db.Where(
		"NOT (type = ? AND COALESCE(token_id, 0) = 0 AND COALESCE(token_name, '') = ? AND COALESCE(content, '') = ?)",
		LogTypeConsume, "模型测试", "模型测试",
	)
}

func isNativeChannelTestLog(logType, tokenId int, tokenName, content string) bool {
	return logType == LogTypeConsume && tokenId == 0 && tokenName == "模型测试" && content == "模型测试"
}
