package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

const trustedTaskSecondsBatch = 200

type upstreamTaskSecondsKey struct {
	userID, channelID int
	taskID            string
}

type upstreamTaskSecondsEvidence struct {
	seconds *decimal.Decimal
	source  string
}

// Read only the exact logged user/channel/task identity. Ambiguous matches are
// unusable; a task ID from another account cannot repair this account's bill.
func resolveUpstreamTaskSeconds(ctx context.Context, keys []upstreamTaskSecondsKey) (map[upstreamTaskSecondsKey]Task, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resolved := make(map[upstreamTaskSecondsKey]Task)
	wanted := make(map[upstreamTaskSecondsKey]bool)
	ids := make([]string, 0, len(keys))
	seen := make(map[string]bool)
	for _, key := range keys {
		wanted[key] = true
		if !seen[key.taskID] {
			ids = append(ids, key.taskID)
			seen[key.taskID] = true
		}
	}
	duplicates := make(map[upstreamTaskSecondsKey]bool)
	for start := 0; start < len(ids); start += trustedTaskSecondsBatch {
		var rows []Task
		if err := DB.WithContext(ctx).Select("id, user_id, channel_id, task_id, status, billing_state, quota, properties, private_data").Where("task_id IN ?", ids[start:min(start+trustedTaskSecondsBatch, len(ids))]).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := upstreamTaskSecondsKey{row.UserId, row.ChannelId, row.TaskID}
			if !wanted[key] {
				continue
			}
			if _, exists := resolved[key]; exists {
				duplicates[key] = true
			}
			resolved[key] = row
		}
	}
	for key := range duplicates {
		delete(resolved, key)
	}
	return resolved, nil
}

// Historical parameter-based bills are verified with the original expression,
// group and contract. A creation budget is never presented as measured usage.
func recoverUpstreamTaskSeconds(task Task, log billingReconciliationLog) upstreamTaskSecondsEvidence {
	async := task.PrivateData.AsyncBilling
	if async == nil || async.State != TaskBillingStateSettled || task.BillingState != TaskBillingStateSettled {
		return upstreamTaskSecondsEvidence{}
	}
	if seconds, ok := trustedTaskSnapshotSeconds(task.PrivateData); ok && task.Status == TaskStatusSuccess && async.Operation == "settle" {
		return upstreamTaskSecondsEvidence{&seconds, "recorded_usage"}
	}
	snap := async.TieredSnapshot
	if snap == nil || async.BillingProbe == nil || async.TargetQuota == nil || snap.TaskUsageBilling || len(snap.UsageUnits) > 0 || len(snap.UsageFacts) > 0 || log.Type != LogTypeConsume || log.Quota < 0 {
		return upstreamTaskSecondsEvidence{}
	}
	if task.Properties.OriginModelName != log.ModelName {
		return upstreamTaskSecondsEvidence{}
	}
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return upstreamTaskSecondsEvidence{}
	}
	expression, err := base64.StdEncoding.DecodeString(billingBreakdownString(other["expr_b64"]))
	if err != nil || string(expression) != snap.ExprString || snap.ExprHash != billingexpr.ExprHashString(snap.ExprString) || BillingStatementExpressionMode(snap.ExprString, nil) != BillingReconciliationModePerSecond {
		return upstreamTaskSecondsEvidence{}
	}
	var probe struct {
		Task map[string]json.RawMessage `json:"_task"`
	}
	if common.Unmarshal(async.BillingProbe.Body, &probe) != nil {
		return upstreamTaskSecondsEvidence{}
	}
	seconds, ok := billingReconciliationFloat(probe.Task["duration_seconds"])
	if !ok || seconds < 0 || seconds > relaycommon.MaxTaskDurationSeconds || math.IsNaN(snap.GroupRatio) || math.IsInf(snap.GroupRatio, 0) || snap.GroupRatio <= 0 {
		return upstreamTaskSecondsEvidence{}
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snap, billingexpr.TokenParams{}, *async.BillingProbe)
	if err != nil || result.Clamp != nil || math.IsNaN(result.ActualQuotaBeforeGroup) || math.IsInf(result.ActualQuotaBeforeGroup, 0) || result.ActualQuotaBeforeGroup < 0 {
		return upstreamTaskSecondsEvidence{}
	}
	quota := result.ActualQuotaAfterGroup
	if bc := task.PrivateData.BillingContext; bc != nil && bc.ContractFact != nil {
		ratio := bc.ContractFact.RatioDecimal()
		if ratio.IsZero() {
			return upstreamTaskSecondsEvidence{}
		}
		var clamp *common.QuotaClamp
		quota, clamp = common.QuotaRoundChecked(decimal.NewFromFloat(result.ActualQuotaBeforeGroup).Mul(decimal.NewFromFloat(snap.GroupRatio)).Mul(ratio).InexactFloat64())
		if clamp != nil {
			return upstreamTaskSecondsEvidence{}
		}
	}
	if quota != log.Quota {
		return upstreamTaskSecondsEvidence{}
	}
	if task.Status == TaskStatusSuccess && async.Operation == "settle" && quota == *async.TargetQuota && quota == task.Quota {
		value := decimal.NewFromFloat(seconds)
		return upstreamTaskSecondsEvidence{&value, "billing_parameters"}
	}
	if task.Status.ShouldRefundOnTerminal() && async.Operation == "refund" && *async.TargetQuota == 0 && task.Quota == 0 {
		return upstreamTaskSecondsEvidence{nil, "refunded_hold"}
	}
	return upstreamTaskSecondsEvidence{}
}

// trustedTaskSnapshotSeconds extracts the frozen completion seconds from one
// task's private data. It requires an actually-reported usage and a single
// second-unit meter with a recorded non-negative value.
func trustedTaskSnapshotSeconds(private TaskPrivateData) (decimal.Decimal, bool) {
	async := private.AsyncBilling
	if async == nil || !async.ActualUsageReported || async.TieredSnapshot == nil {
		return decimal.Decimal{}, false
	}
	snap := async.TieredSnapshot
	units, facts := snap.UsageUnits, snap.UsageFacts
	if len(units) == 0 && len(facts) == 0 {
		return decimal.Decimal{}, false
	}
	meter := ""
	for key, unit := range units {
		if unit == "second" {
			if meter != "" {
				return decimal.Decimal{}, false
			}
			meter = key
		}
	}
	if meter == "" {
		return decimal.Decimal{}, false
	}
	raw, err := common.Marshal(facts[meter])
	if err != nil || len(raw) == 0 {
		return decimal.Decimal{}, false
	}
	value, ok := billingReconciliationFloat(raw)
	if !ok || value < 0 {
		return decimal.Decimal{}, false
	}
	return decimal.NewFromFloat(value), true
}
