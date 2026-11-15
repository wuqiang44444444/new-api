package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

// Refund failures use the existing notification transport and per-user/type
// rate limiter. No provider payload or customer financial detail is sent.
func NotifyVideoFundFailure(summary TaskCreateFundSummary, refundFailed bool) {
	queuedAt := time.Now()
	if !EnqueueMailWork(func() {
		root := model.GetRootUser()
		if root == nil || root.Id <= 0 {
			return
		}
		var details strings.Builder
		rows := [][2]string{
			{"检查时间（北京时间） / Checked at (Beijing time)", queuedAt.In(errorReportLocation()).Format("2006-01-02 15:04:05")},
			{"占款释放失败 / Hold release failures", strconv.Itoa(summary.Failed)},
			{"占款释放待处理 / Blocked hold releases", strconv.Itoa(summary.Blocked)},
		}
		if summary.ScanFailed {
			rows = append(rows, [2]string{"占款扫描 / Hold scan", "扫描失败 / Scan failed"})
		}
		if refundFailed {
			rows = append(rows, [2]string{"退款处理 / Refund processing", "执行失败 / Processing failed"})
		}
		writeOperationsMailRows(&details, rows)
		sendLifecycleEmail(model.MailVideoFunds, queuedAt, root.Id, "video_fund_refund_failure", i18n.MsgMailVideoRefundSubject, i18n.MsgMailVideoRefundBody, details.String())
	}) {
		common.SysError("video refund notification queue is full")
	}
}
