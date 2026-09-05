package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/funcloud"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// FunCloud 上报的 completionTokens 与分辨率面积 × 时长成正比，跨模型一致
// （2026-09-04 渠道 #72 成功任务实测：480p ≈ 10.1k/s、720p ≈ 21.9k/s、1080p ≈ 48.7k/s）。
// 信任校验上限按"实测速率 × 2.4 倍余量 × 冻结请求时长"推导，只用于拒绝协议上不可信的
// 巨量数值；它不复用预扣预算（task_billing_setting.preconsume_tokens），预扣预算只决定
// 提交时的资金暂挂金额，结算始终按实际上报用量执行。
const (
	funCloudTokenRatePerSecond480p   = 30_000
	funCloudTokenRatePerSecond720p   = 60_000
	funCloudTokenRatePerSecond1080p  = 120_000
	funCloudTokenCeilingMaxDuration  = 30
	funCloudTokenCeilingMinTokens    = 100_000
	funCloudTokenCeilingFallbackRate = funCloudTokenRatePerSecond1080p
)

func funCloudTokenRatePerSecond(resolution string) int {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480p":
		return funCloudTokenRatePerSecond480p
	case "720p":
		return funCloudTokenRatePerSecond720p
	case "1080p":
		return funCloudTokenRatePerSecond1080p
	default:
		return funCloudTokenCeilingFallbackRate
	}
}

// funCloudTokenCeiling 推导 Provider 用量证据的合理性上限。durationSeconds <= 0 时按
// Provider 已验证的最大时长 30s 计；结果另设 100k 下限，保证短任务不会被零值上限卡死。
func funCloudTokenCeiling(resolution string, durationSeconds int) int {
	if durationSeconds <= 0 {
		durationSeconds = funCloudTokenCeilingMaxDuration
	}
	if durationSeconds > funCloudTokenCeilingMaxDuration {
		durationSeconds = funCloudTokenCeilingMaxDuration
	}
	ceiling := funCloudTokenRatePerSecond(resolution) * (durationSeconds + 1)
	if ceiling < funCloudTokenCeilingMinTokens {
		return funCloudTokenCeilingMinTokens
	}
	return ceiling
}

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
			Resolution      string `json:"resolution"`
			DurationSeconds int    `json:"duration_seconds"`
			HasVideoInput   *bool  `json:"has_video_input"`
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
		MaxTokens:     funCloudTokenCeiling(probe.Task.Resolution, probe.Task.DurationSeconds),
	}, nil
}
