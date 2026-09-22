package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

// Refund failures use the existing notification transport and per-user/type
// rate limiter. No provider payload or customer financial detail is sent.
func NotifyVideoFundFailure() {
	gopool.Go(func() {
		root := model.GetRootUser()
		if root == nil || root.Id <= 0 {
			return
		}
		sendLifecycleEmail(root.Id, "video_fund_refund_failure", "notify.video_refund.subject", "notify.video_refund.body")
	})
}
