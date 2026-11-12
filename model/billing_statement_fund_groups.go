package model

import (
	"encoding/hex"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type BillingStatementFundGroup struct {
	Key        string `json:"key" gorm:"column:fund_key"`
	Date       string `json:"date"`
	ModelName  string `json:"model_name"`
	RequestID  string `json:"request_id,omitempty"`
	EventCount int64  `json:"event_count"`
	NetQuota   int64  `json:"net_quota"`
}

func billingStatementFundIdentity(log Log, parsed parsedBillingReconciliationLog) string {
	day := time.Unix(log.CreatedAt, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	identity := "log:" + strconv.Itoa(log.Id)
	if parsed.taskID != "" && log.TokenId > 0 {
		identity = "task:" + parsed.taskID
	} else if parsed.refundPreauthLogId > 0 && parsed.refundPreauthMatched {
		identity = "log:" + strconv.FormatInt(parsed.refundPreauthLogId, 10)
	}
	hash := common.Sha256Raw([]byte(day + "\x00" + strconv.Itoa(log.UserId) + "\x00" +
		strconv.Itoa(log.TokenId) + "\x00" + strconv.Itoa(log.ChannelId) + "\x00" + parsed.customerModel + "\x00" + identity))
	return day + "." + hex.EncodeToString(hash)
}
