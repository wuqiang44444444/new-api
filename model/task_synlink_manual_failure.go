package model

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

// SynlinkFailureRecovery identifies one manually verified historical task. The
// evidence is an opaque protected-record reference, never a provider response.
// The caller must authenticate the operator and verify the frozen task identity
// against that evidence. This is not a provider parser or an online admin API.
type SynlinkFailureRecovery struct {
	TaskID                                              string
	UserID, AppID, ChannelID, OperatorID, ExpectedQuota int
	EvidenceRef                                         string
}

// Recovery rejection sentinels contain only static, safe diagnostics. Database
// errors remain distinct so callers can classify them without printing details.
var (
	ErrSynlinkRecoveryEnvironmentMismatch    = errors.New("environment_mismatch: recovery requires shared main and log database")
	ErrSynlinkRecoveryInvalidInput           = errors.New("invalid_input: explicit task, owner, application, channel, operator, held quota and evidence reference are required")
	ErrSynlinkRecoveryOperatorInvalid        = errors.New("operator_invalid: enabled Root operator required")
	ErrSynlinkRecoveryTaskScopeMismatch      = errors.New("task_scope_mismatch: task is missing or outside the recovery scope")
	ErrSynlinkRecoveryFrozenContractMismatch = errors.New("frozen_contract_mismatch: task is outside the frozen Synlink 1.3.3 failure recovery contract")
	ErrSynlinkRecoveryFrozenFundingInvalid   = errors.New("frozen_funding_invalid: unverified frozen funding source")
	ErrSynlinkRecoveryEvidenceConflict       = errors.New("evidence_conflict: previous recovery or funding facts conflict; recheck evidence")
	ErrSynlinkRecoveryQuotaMismatch          = errors.New("quota_mismatch: held quota changed; preview and verify again")
	ErrSynlinkRecoveryTaskStateConflict      = errors.New("task_state_conflict: task state changed; preview and verify again")
	ErrSynlinkRecoveryTaskChanged            = errors.New("task_changed: task changed concurrently; recovery rolled back")
)

var synlinkRecoveryReference = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// RecoverSynlinkFailedTask records the manual terminal decision and its audit in
// one transaction. Funding is left to ApplyTaskBillingTarget. This maintenance
// entry requires main and audit/log storage to be the same database, with normal
// workers stopped. Repeating the same decision can resume interrupted funding.
func RecoverSynlinkFailedTask(scope SynlinkFailureRecovery, apply bool) (*Task, error) {
	if DB == nil || LOG_DB != DB {
		return nil, ErrSynlinkRecoveryEnvironmentMismatch
	}
	if !synlinkRecoveryReference.MatchString(scope.TaskID) || !strings.HasPrefix(scope.TaskID, "task_") ||
		scope.UserID <= 0 || scope.AppID < 0 || scope.ChannelID <= 0 || scope.OperatorID <= 0 || scope.ExpectedQuota <= 0 || scope.ExpectedQuota > math.MaxInt32 ||
		!synlinkRecoveryReference.MatchString(scope.EvidenceRef) {
		return nil, ErrSynlinkRecoveryInvalidInput
	}
	var task Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		var operator User
		if err := tx.Select("id", "username", "role", "status").First(&operator, scope.OperatorID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSynlinkRecoveryOperatorInvalid
			}
			return err
		}
		if operator.Role != common.RoleRootUser || operator.Status != common.UserStatusEnabled {
			return ErrSynlinkRecoveryOperatorInvalid
		}
		query := tx
		if apply {
			query = lockForUpdate(tx)
		}
		if err := query.Where("task_id = ? AND user_id = ? AND app_id = ? AND channel_id = ?", scope.TaskID, scope.UserID, scope.AppID, scope.ChannelID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSynlinkRecoveryTaskScopeMismatch
			}
			return err
		}
		execution, async := task.PrivateData.Execution, task.PrivateData.AsyncBilling
		if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 || execution == nil || execution.TaskPlugin == nil ||
			execution.TaskPlugin.Key != "seedance-link" || execution.TaskPlugin.Version != "1.3.3" || execution.TaskPlugin.APIVersion != 3 ||
			task.PrivateData.UpstreamTaskID == "" || async == nil || task.PrivateData.ResultURL != "" || async.ActualUsageReported {
			return ErrSynlinkRecoveryFrozenContractMismatch
		}
		switch task.PrivateData.BillingSource {
		case "wallet":
		case "subscription":
			if task.PrivateData.SubscriptionId <= 0 {
				return ErrSynlinkRecoveryFrozenFundingInvalid
			}
		default:
			return ErrSynlinkRecoveryFrozenFundingInvalid
		}
		eventID := fmt.Sprintf("synlink-failure:%d", task.ID)
		var audits []AuditLog
		if err := tx.Where("event_id = ?", eventID).Find(&audits).Error; err != nil {
			return err
		}
		if len(audits) > 0 {
			var recorded SynlinkFailureRecovery
			encoded, err := common.Marshal(audits[0].Other.RootInfo["recovery"])
			if err != nil {
				return err
			}
			if common.Unmarshal(encoded, &recorded) != nil || recorded != scope || !audits[0].Success ||
				audits[0].Action != "task.synlink_verified_failure" || task.Status != TaskStatusFailure ||
				async.TargetQuota == nil || *async.TargetQuota != 0 || async.Operation != "refund" ||
				(async.State == TaskBillingStateSettled && task.Quota != 0) || (async.State != TaskBillingStateSettled && task.Quota != scope.ExpectedQuota) {
				return ErrSynlinkRecoveryEvidenceConflict
			}
			return nil
		}
		if task.Quota != scope.ExpectedQuota {
			return ErrSynlinkRecoveryQuotaMismatch
		}
		if task.Status != TaskStatusReconciliationRequired || !strings.Contains(task.FailReason, "untrusted Synlink task identity") ||
			async.State != TaskBillingStatePending || task.BillingState != TaskBillingStatePending || async.TargetQuota != nil {
			return ErrSynlinkRecoveryTaskStateConflict
		}
		if !apply {
			return nil
		}
		// Preserve every frozen identity and price. Only establish the verified
		// terminal observation and a zero billing target; never write balances here.
		task.Status, task.Progress = TaskStatusFailure, "100%"
		task.FinishTime = common.GetTimestamp()
		task.FailReason = "Video generation failed (verified by operator)"
		target := 0
		async.Operation, async.Reason, async.TargetQuota = "refund", task.FailReason, &target
		var err error
		task.Data, err = common.Marshal(map[string]any{"status": "failed", "error": map[string]string{"code": "upstream_task_failed", "message": task.FailReason}})
		if err != nil {
			return err
		}
		result := tx.Model(&task).Where("status = ? AND quota = ?", TaskStatusReconciliationRequired, scope.ExpectedQuota).
			Select("status", "progress", "finish_time", "fail_reason", "data", "private_data").Updates(&task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSynlinkRecoveryTaskChanged
		}
		return tx.Create(&AuditLog{EventId: eventID, UserId: operator.Id, Username: operator.Username, ActorRole: operator.Role,
			CreatedAt: task.FinishTime, Category: AuditCategoryOperation, Action: "task.synlink_verified_failure", AuthMethod: "offline_maintenance", Success: true,
			Content: "Manually verified historical video failure; refund requested", Other: AuditOther{RootInfo: AuditFields{"recovery": scope}}}).Error
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}
