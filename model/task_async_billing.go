package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type TaskBillingState string

const (
	TaskBillingStatePending TaskBillingState = "pending"
	TaskBillingStateSettled TaskBillingState = "settled"
	TaskBillingStateFailed  TaskBillingState = "failed"
	TaskBillingStateDebt    TaskBillingState = "debt"
)

type TaskAsyncBillingContext struct {
	TieredSnapshot  *billingexpr.BillingSnapshot `json:"tiered_snapshot,omitempty"`
	BillingProbe    *billingexpr.RequestInput    `json:"billing_probe,omitempty"`
	EstimatedTokens int                          `json:"estimated_tokens,omitempty"`
	ActualTokens    int                          `json:"actual_tokens,omitempty"`
	State           TaskBillingState             `json:"state,omitempty"`
	Error           string                       `json:"error,omitempty"`
	Attempts        int                          `json:"attempts,omitempty"`
	NextRetryAt     int64                        `json:"next_retry_at,omitempty"`
	Operation       string                       `json:"operation,omitempty"`
	TargetQuota     *int                         `json:"target_quota,omitempty"`
	Reason          string                       `json:"reason,omitempty"`
	QuotaClamp      *common.QuotaClamp           `json:"quota_clamp,omitempty"`
}

func AttachAsyncTaskBilling(privateData *TaskPrivateData, info *relaycommon.RelayInfo, quota int) {
	// 仅 tiered_expr 异步任务启用表达式计费状态机；普通任务返回 nil，沿用原有结算/退款路径，
	// 避免改变 Doubao/Suno 等现有渠道的预扣与结算行为（见方案 §5.2 非表达式模式继续原流程）。
	if info.TieredBillingSnapshot == nil {
		return
	}
	if privateData.BillingContext == nil {
		privateData.BillingContext = &TaskBillingContext{OriginModelName: info.OriginModelName}
	}
	// tiered_expr 任务以表达式价格为唯一事实：不按次计费，也不再叠加内置倍率。
	privateData.BillingContext.PerCallBilling = false
	state := TaskBillingStatePending
	if quota == 0 {
		state = TaskBillingStateSettled
	}
	// 仅持久化计费探针的 Body（_task 字段），不落库请求头，避免 Authorization/Cookie 泄露。
	// 结算时表达式只读 _task body 字段（见方案 §5.4），不依赖 Headers。
	probe := &billingexpr.RequestInput{}
	if info.BillingRequestInput != nil {
		probe.Body = append([]byte(nil), info.BillingRequestInput.Body...)
	}
	privateData.AsyncBilling = &TaskAsyncBillingContext{
		TieredSnapshot:  info.TieredBillingSnapshot,
		BillingProbe:    probe,
		State:           state,
		EstimatedTokens: info.TieredBillingSnapshot.EstimatedCompletionTokens,
	}
}

// deriveBillingState 把 private_data 中的计费状态投影到可索引列。
// 普通任务（无 AsyncBilling）返回空串，自然被补偿扫描的 SQL WHERE 排除，无需回填历史数据。
func deriveBillingState(pd TaskPrivateData) TaskBillingState {
	if pd.AsyncBilling == nil {
		return ""
	}
	return pd.AsyncBilling.State
}

func (t *Task) UpdateBilling() error {
	return DB.Model(t).Updates(map[string]any{
		"quota":         t.Quota,
		"private_data":  t.PrivateData,
		"billing_state": deriveBillingState(t.PrivateData),
	}).Error
}

// GetTerminalTasksPendingBilling returns terminal tasks (SUCCESS/FAILURE) whose
// async billing still needs to be settled or refunded. JSON state is filtered in
// Go so all supported databases use the same portable query.
//
// 终态过滤必须在 SQL 层完成（方案 §15.3 P0-1）：非终态任务 ActualTokens 恒为 0，
// 若进入补偿会被 settleTaskTieredSnapshot 把预扣额当作目标额提前标记 settled，
// 而后续真正终态的差额结算与退款又被 settled 幂等守护吞掉，造成不可自愈的错账。
func GetTerminalTasksPendingBilling(now int64, limit int) []*Task {
	if limit <= 0 {
		limit = 100
	}
	pendingStates := []TaskBillingState{TaskBillingStatePending, TaskBillingStateFailed, TaskBillingStateDebt}
	// status 与 billing_state 均为普通列，三库行为一致，无需方言分支。
	terminalStatuses := []string{TaskStatusSuccess, TaskStatusFailure}
	tasks := make([]*Task, 0, limit)
	batchSize := max(limit*5, 100)
	lastID := int64(0)
	for {
		var candidates []*Task
		// 走 billing_state 索引，仅扫描待补偿任务，避免全表扫描历史终态任务。
		// 普通任务 billing_state 为空，不会命中；终态与 state 过滤均由 SQL 承担。
		if err := DB.Where("id > ? AND billing_state IN ? AND status IN ?", lastID, pendingStates, terminalStatuses).
			Order("id").Limit(batchSize).Find(&candidates).Error; err != nil {
			return nil
		}
		for _, task := range candidates {
			lastID = task.ID
			state := task.PrivateData.AsyncBilling
			if state == nil || state.Attempts >= 10 || state.NextRetryAt > now {
				continue
			}
			tasks = append(tasks, task)
			if len(tasks) == limit {
				return tasks
			}
		}
		if len(candidates) < batchSize {
			return tasks
		}
	}
}

func HasTerminalTasksPendingBilling() bool {
	return len(GetTerminalTasksPendingBilling(time.Now().Unix(), 1)) > 0
}
