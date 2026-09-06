package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// buildTaskVideoDetails 从创建时冻结的事实投影视频详情参数：
// 客户请求快照、计费采用探针与结算事实三个来源互不冒充。
// 缺失记录返回 nil 字段（前端显示“未记录”），不根据当前配置推断。
func buildTaskVideoDetails(task *model.Task) *dto.TaskVideoDetails {
	if task == nil {
		return nil
	}
	details := &dto.TaskVideoDetails{}
	details.Request = buildTaskVideoRequestParams(task.PrivateData.ClientRequest)
	details.Billing = buildTaskVideoBillingParams(task)
	details.Settlement = buildTaskVideoSettlement(task)
	if details.Request == nil && details.Billing == nil && details.Settlement == nil {
		return nil
	}
	return details
}

func buildTaskVideoRequestParams(snapshot *model.TaskClientRequestSnapshot) *dto.TaskVideoRequestParams {
	if snapshot == nil {
		return nil
	}
	params := &dto.TaskVideoRequestParams{}
	if snapshot.Seconds != "" {
		params.Seconds = &dto.TaskVideoTextParam{Value: snapshot.Seconds}
	}
	if snapshot.Resolution != "" {
		params.Resolution = &dto.TaskVideoTextParam{Value: snapshot.Resolution}
	}
	if snapshot.Ratio != "" {
		params.Ratio = &dto.TaskVideoTextParam{Value: snapshot.Ratio}
	}
	if snapshot.GenerateAudio != nil {
		params.GenerateAudio = &dto.TaskVideoBoolParam{Value: *snapshot.GenerateAudio}
	}
	if snapshot.ServiceTier != "" {
		params.ServiceTier = &dto.TaskVideoTextParam{Value: snapshot.ServiceTier}
	}
	if params.Seconds == nil && params.Resolution == nil && params.Ratio == nil &&
		params.GenerateAudio == nil && params.ServiceTier == nil {
		return nil
	}
	return params
}

// buildTaskVideoBillingParams 读取创建时冻结的计费探针正文。探针只在
// tiered 计费快照存在时持久化，属于“计费采用参数”：可能包含默认值与
// 智能时长上限换算，不得当作客户显式传值或南向发送内容展示。
func buildTaskVideoBillingParams(task *model.Task) *dto.TaskVideoBillingParams {
	asyncBilling := task.PrivateData.AsyncBilling
	if asyncBilling == nil || asyncBilling.BillingProbe == nil || len(asyncBilling.BillingProbe.Body) == 0 {
		return nil
	}
	var envelope struct {
		Task struct {
			DurationSeconds any    `json:"duration_seconds"`
			Resolution      string `json:"resolution"`
			GenerateAudio   *bool  `json:"generate_audio"`
		} `json:"_task"`
	}
	if err := common.Unmarshal(asyncBilling.BillingProbe.Body, &envelope); err != nil {
		return nil
	}
	probeBody := envelope.Task
	params := &dto.TaskVideoBillingParams{}
	switch value := probeBody.DurationSeconds.(type) {
	case string:
		if value != "" {
			params.DurationSeconds = &dto.TaskVideoTextParam{Value: value}
		}
	case float64:
		params.DurationSeconds = &dto.TaskVideoTextParam{Value: strconv.FormatFloat(value, 'f', -1, 64)}
	}
	if probeBody.Resolution != "" {
		params.Resolution = &dto.TaskVideoTextParam{Value: probeBody.Resolution}
	}
	if probeBody.GenerateAudio != nil {
		params.GenerateAudio = &dto.TaskVideoBoolParam{Value: *probeBody.GenerateAudio}
	}
	if params.DurationSeconds == nil && params.Resolution == nil && params.GenerateAudio == nil {
		return nil
	}
	return params
}

func buildTaskVideoSettlement(task *model.Task) *dto.TaskVideoSettlement {
	asyncBilling := task.PrivateData.AsyncBilling
	settlement := &dto.TaskVideoSettlement{
		Quota: task.Quota,
	}
	hasFacts := false
	if task.Quota != 0 {
		hasFacts = true
	}
	if asyncBilling != nil {
		if asyncBilling.State != "" {
			settlement.BillingState = string(asyncBilling.State)
			hasFacts = true
		}
		if asyncBilling.ActualUsageReported && len(asyncBilling.ActualUsageEvidence) > 0 {
			settlement.ActualUsageReported = true
			settlement.ActualUsage = asyncBilling.ActualUsageEvidence
			hasFacts = true
		}
	}
	if billingContext := task.PrivateData.BillingContext; billingContext != nil && len(billingContext.OtherRatios) > 0 {
		settlement.OtherRatios = billingContext.OtherRatios
		hasFacts = true
	}
	if !hasFacts {
		return nil
	}
	return settlement
}
