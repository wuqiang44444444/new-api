package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/funcloud"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// funCloudTaskResponseContext 从创建时冻结的计费上下文推导 FunCloud 终态响应事实。
// 缺失事实时失败关闭：没有冻结探针就无法归一化 FunCloud 终态响应。
func funCloudTaskResponseContext(billingContext *relaycommon.VideoTaskBillingContext) (funcloud.TaskResponseContext, error) {
	violation := func() (funcloud.TaskResponseContext, error) {
		return funcloud.TaskResponseContext{}, &relaycommon.UpstreamContractViolation{Reason: "FunCloud billing context is invalid"}
	}
	if billingContext == nil || strings.TrimSpace(billingContext.ProviderModel) == "" ||
		len(billingContext.BillingProbeBody) == 0 || billingContext.EstimatedTokens <= 0 {
		return violation()
	}
	var probe struct {
		Task struct {
			Resolution    string `json:"resolution"`
			HasVideoInput *bool  `json:"has_video_input"`
		} `json:"_task"`
	}
	if common.Unmarshal(billingContext.BillingProbeBody, &probe) != nil ||
		probe.Task.HasVideoInput == nil || strings.TrimSpace(probe.Task.Resolution) == "" {
		return violation()
	}
	return funcloud.TaskResponseContext{
		ProviderModel: strings.TrimSpace(billingContext.ProviderModel),
		Resolution:    strings.ToLower(strings.TrimSpace(probe.Task.Resolution)),
		HasVideoInput: *probe.Task.HasVideoInput,
		MaxTokens:     billingContext.EstimatedTokens,
	}, nil
}
