package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
)

// Refund failures use the existing notification transport and per-user/type
// rate limiter. No provider payload or customer financial detail is sent.
func NotifyVideoFundFailure() {
	if !EnqueueMailWork(func() {
		root := model.GetRootUser()
		if root == nil || root.Id <= 0 {
			return
		}
		sendLifecycleEmail(root.Id, "video_fund_refund_failure", i18n.MsgMailVideoRefundSubject, i18n.MsgMailVideoRefundBody)
	}) {
		common.SysError("video refund notification queue is full")
	}
}
