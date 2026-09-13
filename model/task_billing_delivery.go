package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// TaskBillingDelivery is a log delivery checkpoint, never a funding ledger.
// Its immutable amounts are copied in the Task funding transaction. Replaying
// it only projects a log and statistics; it cannot debit or refund an account.
type TaskBillingDelivery struct {
	ID               int64  `gorm:"primaryKey"`
	TaskRowID        int64  `gorm:"uniqueIndex:idx_task_billing_delivery,priority:1;index"`
	Event            string `gorm:"type:varchar(20);uniqueIndex:idx_task_billing_delivery,priority:2"`
	BeforeQuota      int
	AfterQuota       int
	CompletionTokens int
	UsageReported    bool
	SuppressLog      bool
	CreatedAt        int64
	DeliveredAt      int64 `gorm:"index:idx_task_billing_delivery_pending,priority:1"`
	NextRetryAt      int64 `gorm:"index:idx_task_billing_delivery_pending,priority:2"`
	Attempts         int
}

func UsesTaskBillingDelivery(task *Task) bool {
	// Batch owns its durable log acknowledgement in CompleteBatchSettlement.
	return task != nil && task.Platform != constant.TaskPlatformAzureBatch && task.PrivateData.AsyncBilling != nil && (!task.HasTaskUsageBilling() || task.HasSeedanceBillingFacts())
}

func InsertTaskWithBillingLogContext(ctx context.Context, task *Task) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return createTaskAndBillingLogTx(tx, task) })
}

func createTaskAndBillingLogTx(tx *gorm.DB, task *Task) error {
	if err := tx.Create(task).Error; err != nil {
		return err
	}
	if !UsesTaskBillingDelivery(task) || IsImageTask(task) {
		return nil
	}
	return queueTaskBillingDeliveryTx(tx, task, "create", 0, task.Quota)
}

func queueTaskBillingDeliveryTx(tx *gorm.DB, task *Task, event string, before, after int) error {
	if !UsesTaskBillingDelivery(task) {
		return nil
	}
	async := task.PrivateData.AsyncBilling
	row := TaskBillingDelivery{TaskRowID: task.ID, Event: event, BeforeQuota: before, AfterQuota: after,
		CreatedAt: common.GetTimestamp(), CompletionTokens: async.ActualTokens, UsageReported: async.ActualUsageReported}
	if event == "create" {
		row.CompletionTokens, row.UsageReported = 0, false
		if task.SubmitTime > 0 {
			row.CreatedAt = task.SubmitTime
		}
	}
	// Give new work an actual due time as well: a continuous stream of zero
	// timestamps would otherwise starve older retries. Existing zero values
	// are simply already-due work and drain first.
	row.NextRetryAt = row.CreatedAt
	row.SuppressLog = !common.LogConsumeEnabled && event != "refund" && !(event == "adjustment" && after < before)
	return tx.Create(&row).Error
}

const taskBillingDeliveryTimeout = 5 * time.Second

// DeliverTaskBillingLog serializes one event on the main DB row. The stable
// request ID detects a successful log write whose main-DB acknowledgement was
// lost. Main-DB statistics and the delivery acknowledgement commit together.
func DeliverTaskBillingLog(ctx context.Context, id int64, build func(*Task, TaskBillingDelivery) (*Log, error)) error {
	ctx, cancel := context.WithTimeout(ctx, taskBillingDeliveryTimeout)
	defer cancel()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// This write also obtains SQLite's writer lock, where FOR UPDATE is absent.
		if err := tx.Model(&TaskBillingDelivery{}).Where("id = ? AND delivered_at = 0", id).
			UpdateColumn("attempts", gorm.Expr("attempts + 1")).Error; err != nil {
			return err
		}
		var event TaskBillingDelivery
		if err := lockForUpdate(tx).First(&event, id).Error; err != nil {
			return err
		}
		if event.DeliveredAt != 0 {
			return nil
		}
		var earlier int64
		if err := tx.Model(&TaskBillingDelivery{}).Where("task_row_id = ? AND id < ? AND delivered_at = 0", event.TaskRowID, event.ID).Count(&earlier).Error; err != nil {
			return err
		}
		if earlier > 0 {
			return errors.New("earlier task log is not delivered")
		}
		var task Task
		if err := tx.First(&task, event.TaskRowID).Error; err != nil {
			return err
		}
		log, err := build(&task, event)
		if err != nil {
			return err
		}
		log.RequestId = fmt.Sprintf("task-billing:%d:%s", task.ID, event.Event)
		log.CreatedAt = event.CreatedAt
		var user User
		if err := tx.Select("id", "username").First(&user, task.UserId).Error; err != nil {
			return err
		}
		log.Username = user.Username
		if log.TokenId > 0 {
			var token Token
			err := tx.Unscoped().Select("id", "name").First(&token, log.TokenId).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			log.TokenName = token.Name
		}
		if !event.SuppressLog {
			logDB := LOG_DB.WithContext(ctx)
			if LOG_DB == DB {
				logDB = tx
			}
			var existing []Log
			if err := logDB.Where("request_id = ?", log.RequestId).Limit(2).Find(&existing).Error; err != nil {
				return err
			}
			if len(existing) > 1 {
				return errors.New("duplicate task billing log")
			}
			if len(existing) == 1 {
				old := existing[0]
				if old.UserId != log.UserId || old.TokenId != log.TokenId || old.ChannelId != log.ChannelId || old.Type != log.Type || old.Quota != log.Quota || old.CompletionTokens != log.CompletionTokens || old.PromptTokens != log.PromptTokens {
					return errors.New("task billing log conflicts with frozen event")
				}
			} else if err := logDB.Create(log).Error; err != nil {
				return err
			}
		}
		data := taskLogQuotaData(log, task.PrivateData.NodeName)
		// Image completion already commits these counters with its slot release.
		if !IsImageTask(&task) {
			if err := tx.Model(&User{}).Where("id = ?", task.UserId).Updates(map[string]any{
				"used_quota": gorm.Expr("used_quota + ?", data.Quota), "request_count": gorm.Expr("request_count + ?", data.Count),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", data.Quota)).Error; err != nil {
				return err
			}
		}
		if common.DataExportEnabled && !event.SuppressLog {
			// QuotaData is an additive projection; separate rows are already summed
			// by all readers. Inserting here avoids a second cache acknowledgement.
			if err := tx.Create(data).Error; err != nil {
				return err
			}
		}
		return tx.Model(&event).Updates(map[string]any{"delivered_at": common.GetTimestamp(), "next_retry_at": 0}).Error
	})
}

func PendingTaskBillingDeliveries(ctx context.Context, taskRowID int64, limit int) ([]TaskBillingDelivery, error) {
	ctx, cancel := context.WithTimeout(ctx, taskBillingDeliveryTimeout)
	defer cancel()
	var events []TaskBillingDelivery
	query := DB.WithContext(ctx).Where("delivered_at = 0")
	if taskRowID > 0 {
		query = query.Where("task_row_id = ?", taskRowID)
	} else {
		query = query.Where("next_retry_at <= ?", common.GetTimestamp())
		// A failed early ID must yield to older due work, even when a polling
		// pass takes longer than the retry delay. Per-Task order is enforced
		// again under the delivery lock, independent of this global ordering.
		query = query.Order("next_retry_at")
	}
	err := query.Order("id").Limit(limit).Find(&events).Error
	return events, err
}

func DeferTaskBillingDelivery(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, taskBillingDeliveryTimeout)
	defer cancel()
	return DB.WithContext(ctx).Model(&TaskBillingDelivery{}).Where("id = ? AND delivered_at = 0", id).Update("next_retry_at", common.GetTimestamp()+30).Error
}
