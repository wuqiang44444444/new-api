package controller

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// evidenceRetentionHandler 周期清理过期证据正文：按事件 ID 对象命名规则删除
// 对象、回收安全范围内的空目录，再分批删除明细及到期索引。清理不设关闭
// 开关：TASK_REQUEST_EVIDENCE_ENABLED=false 只停止新增采集，不停止存量回收；
// 不显示成从未收到请求，也不影响任务、资金或对账事实。
type evidenceRetentionHandler struct{}

func (evidenceRetentionHandler) Type() string {
	return model.SystemTaskTypeTaskRequestEvidenceRetention
}

// 清理默认工作：随应用运行，每小时一轮；未启用采集的部署同样清理存量。
func (evidenceRetentionHandler) Enabled() bool { return true }

func (evidenceRetentionHandler) Interval() time.Duration { return time.Hour }

func (evidenceRetentionHandler) NewPayload() any { return nil }

// evidenceRetentionStats 是每次清理的结果事实，写入 SystemTask 历史 result。
// 只含计数与净化错误类别，不含正文、密钥、完整 URL 或 Task 私有数据。
type evidenceRetentionStats struct {
	StartedAt         int64  `json:"started_at"`
	FinishedAt        int64  `json:"finished_at"`
	Cutoff            int64  `json:"cutoff"`
	RetentionDays     int    `json:"retention_days"`
	Trigger           string `json:"trigger"`
	TargetTimestamp   int64  `json:"target_timestamp,omitempty"`
	OperatorUserID    int    `json:"operator_user_id,omitempty"`
	Scanned           int64  `json:"scanned"`
	CleanedIndexes    int64  `json:"cleaned_indexes"`
	DeletedFiles      int64  `json:"deleted_files"`
	FreedBytes        int64  `json:"freed_bytes"`
	MissingFiles      int64  `json:"missing_files"`
	FailedItems       int64  `json:"failed_items"`
	UnattributedFiles int64  `json:"unattributed_files"`
	DeletedEvents     int64  `json:"deleted_events"`
	DeletedIndexes    int64  `json:"deleted_indexes"`
	DeletedAccessLogs int64  `json:"deleted_access_logs"`
	DeletedHistory    int64  `json:"deleted_history"`
	DatabaseFailures  int64  `json:"database_failures"`
	ErrorCategory     string `json:"error_category,omitempty"`
}

func (evidenceRetentionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	stats := evidenceRetentionStats{StartedAt: common.GetTimestamp()}
	stats.Cutoff = stats.StartedAt
	persistProgress := func() error { return model.UpdateSystemTaskState(task.TaskID, runnerID, stats) }
	retentionSeconds := service.EvidenceRetentionSeconds()
	stats.RetentionDays = int(retentionSeconds / 86400)
	// 手动运行携带固定截止与操作者；执行与重试始终使用原有边界，不按
	// “多少天前”重新计算。调度运行的 payload 为空，按七天滚动处理。
	payload := service.EvidenceCleanupPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		stats.ErrorCategory = "invalid_payload"
		finishEvidenceRetention(task, runnerID, stats, err)
		return
	}
	if payload.TargetTimestamp < 0 || (payload.TargetTimestamp > stats.StartedAt) {
		stats.ErrorCategory = "invalid_payload"
		finishEvidenceRetention(task, runnerID, stats, errors.New(stats.ErrorCategory))
		return
	}
	stats.Trigger = payload.Trigger
	if stats.Trigger == "" {
		stats.Trigger = "scheduled"
	}
	stats.TargetTimestamp = payload.TargetTimestamp
	stats.OperatorUserID = payload.OperatorUserID
	if stats.TargetTimestamp == 0 {
		if err := cleanupEvidenceAuditAndHistory(ctx, &stats, persistProgress); err != nil {
			if errors.Is(err, model.ErrSystemTaskLockLost) {
				return
			}
			stats.ErrorCategory = "database_cleanup_failed"
			if errors.Is(err, context.Canceled) {
				stats.ErrorCategory = "cancelled"
			} else {
				stats.DatabaseFailures++
			}
			finishEvidenceRetention(task, runnerID, stats, err)
			return
		}
	}

	store := service.GetTaskRequestEvidenceStore()
	if store == nil {
		stats.ErrorCategory = "store_unavailable"
		finishEvidenceRetention(task, runnerID, stats, errors.New(stats.ErrorCategory))
		return
	}

	const batchSize = 100
	var cursor int64
	for {
		if ctx.Err() != nil {
			stats.FinishedAt = common.GetTimestamp()
			_ = model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, stats, "cancelled")
			return
		}
		expired, err := model.TaskRequestEvidenceExpiredBodies(stats.Cutoff, retentionSeconds, stats.TargetTimestamp, cursor, batchSize)
		if err != nil {
			stats.ErrorCategory = "scan_failed"
			finishEvidenceRetention(task, runnerID, stats, err)
			return
		}
		if len(expired) == 0 {
			break
		}
		for _, evidence := range expired {
			stats.Scanned++
			if !processEvidenceRetentionItem(ctx, store, evidence, &stats) {
				stats.FailedItems++
			}
			cursor = evidence.Id
		}
		stateErr := model.UpdateSystemTaskState(task.TaskID, runnerID, stats)
		if stateErr != nil {
			// 租约丢失：停止本轮，任务保持运行态等待既有过期租约恢复调度。
			return
		}
		if len(expired) < batchSize {
			break
		}
	}
	if err := cleanupEvidenceDatabase(ctx, store, &stats, persistProgress); err != nil {
		if errors.Is(err, model.ErrSystemTaskLockLost) {
			return
		}
		stats.ErrorCategory = "database_cleanup_failed"
		if errors.Is(err, context.Canceled) {
			stats.ErrorCategory = "cancelled"
		} else {
			stats.DatabaseFailures++
		}
		finishEvidenceRetention(task, runnerID, stats, err)
		return
	}
	stats.FinishedAt = common.GetTimestamp()
	if stats.FailedItems > 0 {
		if stats.ErrorCategory == "" {
			stats.ErrorCategory = "delete_failed"
		}
		finishEvidenceRetention(task, runnerID, stats, errors.New(stats.ErrorCategory))
		return
	}
	finishEvidenceRetention(task, runnerID, stats, nil)
}

// processEvidenceRetentionItem deletes the objects of one candidate evidence.
// 持久清理申请先落库：读取与追加立即关闭，失败重试与完成之间不会重新写入。
// Objects are located by the event-ID naming rule even when object_key is
// still empty, covering the reserved-but-uncommitted window. Missing objects
// count as idempotent success and are reported separately; the index is marked
// BodyExpired only after every object deletion succeeded.
func processEvidenceRetentionItem(ctx context.Context, store service.TaskRequestEvidenceObjectStore, evidence *model.TaskRequestEvidence, stats *evidenceRetentionStats) bool {
	if evidence.CleanupRequestedAt == 0 {
		if err := model.RequestTaskRequestEvidenceCleanup(evidence.Id, common.GetTimestamp()); err != nil {
			stats.ErrorCategory = "mark_failed"
			return false
		}
	}
	if evidence.BodyExpired {
		// A missing/empty namespace confirms completion without counting previously
		// removed objects again. Legacy residual files still take the full path.
		if err := store.RemoveEmptyEvidenceDir(evidence.Id); err == nil {
			return true
		}
	}

	if !deleteEvidenceObjects(ctx, store, evidence.Id, stats) {
		return false
	}

	if err := model.MarkTaskRequestEvidenceBodyExpired(evidence.Id); err != nil {
		stats.ErrorCategory = "mark_failed"
		return false
	}
	stats.CleanedIndexes++
	return true
}

// finishEvidenceRetention commits the terminal state. A run with any failed
// item finishes failed so the next scheduled run retries; a clean run finishes
// succeeded. The result payload carries the sanitized stats only.
func finishEvidenceRetention(task *model.SystemTask, runnerID string, stats evidenceRetentionStats, runErr error) {
	stats.FinishedAt = common.GetTimestamp()
	message := ""
	if runErr != nil {
		message = stats.ErrorCategory
		if message == "" {
			message = "cleanup_failed"
		}
		logger.LogWarn(context.Background(), "evidence cleanup failed: "+message)
	}
	status := model.SystemTaskStatusSucceeded
	if runErr != nil || stats.FailedItems > 0 {
		status = model.SystemTaskStatusFailed
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, stats, message); err != nil {
		common.SysError("evidence retention finish failed: " + err.Error())
	}
}

func deleteEvidenceObjects(ctx context.Context, store service.TaskRequestEvidenceObjectStore, evidenceID int64, stats *evidenceRetentionStats) bool {
	ok := true
	var cursor int64
	for {
		if ctx.Err() != nil {
			stats.ErrorCategory = "cancelled"
			return false
		}
		events, err := model.TaskRequestEvidenceEventBatch(evidenceID, cursor, 500)
		if err != nil {
			stats.ErrorCategory = "list_events_failed"
			return false
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			size, deleteErr := store.Delete(service.EvidenceObjectKey(evidenceID, event.Id))
			if deleteErr != nil {
				ok = false
				stats.ErrorCategory = "delete_failed"
			} else if size > 0 {
				stats.DeletedFiles++
				stats.FreedBytes += size
			} else {
				stats.MissingFiles++
			}
			cursor = event.Id
		}
		if len(events) < 500 {
			break
		}
	}

	if !ok {
		return false
	}
	// The accepted namespace is closed. Reclaim every remaining body/staging
	// file even when its event row is missing or came from a different backup.
	if err := store.WalkEvidenceFiles(ctx, evidenceID, func(file service.EvidenceStoredFile) error {
		size, err := store.Delete(file.Key)
		if err != nil {
			return err
		}
		if size > 0 {
			stats.DeletedFiles++
			stats.FreedBytes += size
		} else {
			stats.MissingFiles++
		}
		return nil
	}); err != nil {
		stats.ErrorCategory = "delete_failed"
		if ctx.Err() != nil {
			stats.ErrorCategory = "cancelled"
		}
		return false
	}
	if err := store.RemoveEmptyEvidenceDir(evidenceID); err != nil {
		stats.ErrorCategory = "directory_cleanup_failed"
		var nonEmpty *service.EvidenceDirectoryNotEmptyError
		if errors.As(err, &nonEmpty) {
			stats.UnattributedFiles += int64(nonEmpty.Files)
		}
		return false
	}
	return true
}
