package billing_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

// Batch billing keeps a dedicated expression per model. Batch price selection
// lives behind these accessors so normal model price keys never change
// meaning; a model without a batch expression is not batch-billable and must
// be rejected instead of silently reusing the normal price.

const (
	BatchBillingMode          = "batch_expr"
	BatchBillingExprField     = "batch_billing_expr"
)

// BatchBillingSetting is managed by config.GlobalConfig.Register.
// DB key: billing_setting.batch_billing_expr
type BatchBillingSetting struct {
	BatchBillingExpr map[string]string `json:"batch_billing_expr"`
}

var batchBillingSetting = BatchBillingSetting{
	BatchBillingExpr: make(map[string]string),
}

func init() {
	config.GlobalConfig.Register("batch_billing_setting", &batchBillingSetting)
}

// GetBatchBillingExpr returns the configured batch expression for one model.
func GetBatchBillingExpr(model string) (string, bool) {
	expr, ok := batchBillingSetting.BatchBillingExpr[model]
	if !ok || expr == "" {
		return "", false
	}
	return expr, true
}

func GetBatchBillingExprCopy() map[string]string {
	return lo.Assign(batchBillingSetting.BatchBillingExpr)
}
