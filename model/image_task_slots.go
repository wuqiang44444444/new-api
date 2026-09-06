package model

import (
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImageTaskSlot 是图片任务的受理/执行容量计数行。计数只保存可由 Task
// 重建的占用（§3.10 一致性）；RebuildImageTaskSlots 周期性对账。
type ImageTaskSlot struct {
	Scope     string `json:"-" gorm:"primaryKey;type:varchar(96)"`
	Count     int    `json:"-"`
	UpdatedAt int64  `json:"-"`
}

var errImageSlotLimit = errors.New("image task capacity limit reached")

// slotLimitError 携带具体 scope，供受理层映射 429/503。
type slotLimitError struct{ scope string }

func (e *slotLimitError) Error() string { return "image task capacity limit reached: " + e.scope }

func IsImageSlotLimitError(err error) bool {
	var limitErr *slotLimitError
	return errors.As(err, &limitErr)
}

func IsImageSlotAppLimit(err error) bool {
	var limitErr *slotLimitError
	return errors.As(err, &limitErr) && limitErr.scope == "app"
}

// reserveImageSlotsTx locks（FOR UPDATE）与创建 scope 计数行，校验上限后
// 原子递增。scope 按字典序加锁，避免交叉死锁。
func reserveImageSlotsTx(tx *gorm.DB, scopeA string, limitA int, scopeB string, limitB int) error {
	scopes := make([]string, 0, 2)
	limits := map[string]int{}
	if scopeA != "" {
		scopes = append(scopes, scopeA)
		limits[scopeA] = limitA
	}
	if scopeB != "" && scopeB != scopeA {
		scopes = append(scopes, scopeB)
		limits[scopeB] = limitB
	}
	if len(scopes) == 0 {
		return nil
	}
	sort.Strings(scopes)

	// 先幂等建立计数行，再统一加锁读取；行存在性不受并发插入影响。
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&[]ImageTaskSlot{
			{Scope: scopes[0], Count: 0, UpdatedAt: common.GetTimestamp()},
			{Scope: scopes[len(scopes)-1], Count: 0, UpdatedAt: common.GetTimestamp()},
		}).Error; err != nil {
		return err
	}
	var slots []ImageTaskSlot
	if err := lockForUpdate(tx).Where("scope IN ?", scopes).Order("scope").Find(&slots).Error; err != nil {
		return err
	}
	counts := make(map[string]int, len(slots))
	for _, slot := range slots {
		counts[slot.Scope] = slot.Count
	}
	for _, scope := range scopes {
		limit := limits[scope]
		if limit <= 0 {
			continue // 未配置上限的 scope 只计数不限制
		}
		if counts[scope] >= limit {
			if scope == "accept:global" {
				return &slotLimitError{scope: "global"}
			}
			if len(scope) > len("accept:app:") && scope[:len("accept:app:")] == "accept:app:" {
				return &slotLimitError{scope: "app"}
			}
			return &slotLimitError{scope: "exec"}
		}
	}
	for _, scope := range scopes {
		if err := tx.Model(&ImageTaskSlot{}).Where("scope = ?", scope).
			Updates(map[string]any{"count": gorm.Expr("count + 1"), "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
	}
	return nil
}

// RebuildImageTaskSlots 在单一事务内重建全部容量计数：先按 scope 字典序
// 锁定既有计数行（与受理/领取的加锁顺序一致），再在事务内统计 Task 事实
// 并写入/归零。受理与领取事务同样先锁计数行再插任务，因此统计与覆盖不
// 存在无协调交错（评审 S9）；已消失的 scope 显式归零。
func RebuildImageTaskSlots() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockImageTaskSlotsTx(tx); err != nil {
			return err
		}
		var locked []ImageTaskSlot
		if err := lockForUpdate(tx).Order("scope").Find(&locked).Error; err != nil {
			return err
		}

		counts, err := countImageTaskSlotsTx(tx)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		for _, slot := range locked {
			target := counts[slot.Scope]
			delete(counts, slot.Scope)
			if slot.Count == target {
				continue
			}
			if err := tx.Model(&ImageTaskSlot{}).Where("scope = ?", slot.Scope).
				Updates(map[string]any{"count": target, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		for scope, total := range counts {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "scope"}},
				DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
			}).Create(&ImageTaskSlot{Scope: scope, Count: total, UpdatedAt: now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func countImageTaskSlotsTx(tx *gorm.DB) (map[string]int, error) {
	counts := make(map[string]int)
	terminal := TerminalTaskStatuses()
	imageProtocol := TaskClientProtocolImageOpenAIV1

	var acceptGlobal, execGlobal int64
	if err := tx.Model(&Task{}).
		Where("client_protocol = ? AND (status NOT IN ? OR billing_state IN ?)", imageProtocol, terminal, []TaskBillingState{TaskBillingStatePending, TaskBillingStateFailed, TaskBillingStateDebt}).
		Count(&acceptGlobal).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&Task{}).
		Where("client_protocol = ? AND status = ?", imageProtocol, TaskStatusInProgress).
		Count(&execGlobal).Error; err != nil {
		return nil, err
	}
	counts[ImageTaskAdmissionScopeGlobal()] = int(acceptGlobal)
	counts[ImageTaskExecutionScopeGlobal()] = int(execGlobal)

	type groupCount struct {
		Owner  int
		Owner2 int
		Total  int64
	}
	var appGroups []groupCount
	if err := tx.Model(&Task{}).
		Select("user_id AS owner, app_id AS owner2, COUNT(*) AS total").
		Where("client_protocol = ? AND (status NOT IN ? OR billing_state IN ?)", imageProtocol, terminal, []TaskBillingState{TaskBillingStatePending, TaskBillingStateFailed, TaskBillingStateDebt}).
		Group("user_id, app_id").Scan(&appGroups).Error; err != nil {
		return nil, err
	}
	for _, group := range appGroups {
		counts[fmt.Sprintf("accept:app:%d:%d", group.Owner, group.Owner2)] = int(group.Total)
	}

	var channelGroups []groupCount
	if err := tx.Model(&Task{}).
		Select("channel_id AS owner, 0 AS owner2, COUNT(*) AS total").
		Where("client_protocol = ? AND status = ?", imageProtocol, TaskStatusInProgress).
		Group("channel_id").Scan(&channelGroups).Error; err != nil {
		return nil, err
	}
	for _, group := range channelGroups {
		counts[fmt.Sprintf("exec:channel:%d", group.Owner)] = int(group.Total)
	}
	return counts, nil
}

// lockImageTaskSlotsTx serializes image capacity mutations and reconciliation.
// Always lock this existing global admission row before a Task or other scope.
func lockImageTaskSlotsTx(tx *gorm.DB) error {
	row := ImageTaskSlot{Scope: ImageTaskAdmissionScopeGlobal()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	return lockForUpdate(tx).Where("scope = ?", row.Scope).First(&row).Error
}
func imageAdmissionOccupied(task *Task) bool {
	return task.Status.IsActive() || (task.PrivateData.AsyncBilling != nil && task.PrivateData.AsyncBilling.State != TaskBillingStateSettled)
}

// Caller holds the global image guard; update slots in the same transaction as Task.
func updateImageTaskSlotsTx(tx *gorm.DB, before, after *Task) error {
	scopes := []string{}
	if before.Status == TaskStatusInProgress && after.Status != TaskStatusInProgress {
		scopes = append(scopes, ImageTaskExecutionScopeGlobal(), ImageTaskExecutionScopeChannel(before.ChannelId))
	}
	if imageAdmissionOccupied(before) && !imageAdmissionOccupied(after) {
		scopes = append(scopes, ImageTaskAdmissionScopeGlobal(), ImageTaskAdmissionScopeApp(before.UserId, before.AppID))
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		if err := tx.Model(&ImageTaskSlot{}).Where("scope = ? AND count > 0", scope).
			Updates(map[string]any{"count": gorm.Expr("count - 1"), "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
	}
	return nil
}

// Shared billing takes this narrow image-only lock before locking the Task.
func lockImageTaskBillingTx(tx *gorm.DB, task *Task) error {
	if !IsImageTask(task) {
		return nil
	}
	return lockImageTaskSlotsTx(tx)
}

func completeImageTaskBillingTx(tx *gorm.DB, task *Task) error {
	if !IsImageTask(task) {
		return nil
	}
	before := *task
	before.PrivateData = task.PrivateData
	before.PrivateData.AsyncBilling = &TaskAsyncBillingContext{State: TaskBillingStatePending}
	if err := updateImageTaskSlotsTx(tx, &before, task); err != nil {
		return err
	}
	if err := tx.Model(&User{}).Where("id = ?", task.UserId).Updates(map[string]any{
		"used_quota": gorm.Expr("used_quota + ?", task.Quota), "request_count": gorm.Expr("request_count + 1"),
	}).Error; err != nil {
		return err
	}
	return tx.Model(&Channel{}).Where("id = ?", task.ChannelId).Update("used_quota", gorm.Expr("used_quota + ?", task.Quota)).Error
}
