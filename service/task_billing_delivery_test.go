package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskBillingDeliveryRecoversWithoutFundingReplay(t *testing.T) {
	for _, failure := range []string{"none", "log write", "ack after split log write"} {
		t.Run(failure, func(t *testing.T) {
			truncate(t)
			oldExport := common.DataExportEnabled
			common.DataExportEnabled = true
			t.Cleanup(func() { common.DataExportEnabled = oldExport; model.DB.Exec("DELETE FROM quota_data") })
			oldLogs := model.LOG_DB
			if failure == "ack after split log write" {
				db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
				require.NoError(t, db.AutoMigrate(&model.Log{}))
				model.LOG_DB = db
				t.Cleanup(func() { model.LOG_DB = oldLogs; sqlDB.Close() })
			}
			seedUser(t, 9919, 900)
			task := makeTask(9919, 0, 100, 0, BillingSourceWallet, 0)
			task.TaskID = model.GenerateTaskID()
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
			require.NoError(t, task.InsertWithContext(context.Background()))
			DeliverTaskBillingLogs(context.Background(), task.ID, 10)
			task.PrivateData.AsyncBilling.ActualTokens = 80
			task.PrivateData.AsyncBilling.ActualUsageReported = true
			task.PrivateData.AsyncBilling.Operation = "settle"
			require.NoError(t, model.DB.Save(task).Error)
			applied, delta, err := model.ApplyTaskBillingTarget(task, 80)
			require.NoError(t, err)
			require.True(t, applied)
			require.Equal(t, -20, delta)
			var pending []model.TaskBillingDelivery
			pending, err = model.PendingTaskBillingDeliveries(context.Background(), task.ID, 10)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			if failure == "log write" {
				require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register("test:delivery-log-fail", func(tx *gorm.DB) {
					if tx.Statement.Table == "logs" {
						tx.AddError(errors.New("simulated log write failure"))
					}
				}))
				require.Error(t, model.DeliverTaskBillingLog(context.Background(), pending[0].ID, BuildTaskBillingDeliveryLog))
				require.NoError(t, model.LOG_DB.Callback().Create().Remove("test:delivery-log-fail"))
			}
			if failure == "ack after split log write" {
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("test:delivery-ack-fail", func(tx *gorm.DB) {
					if tx.Statement.Table == "task_billing_deliveries" {
						if updates, ok := tx.Statement.Dest.(map[string]any); ok && updates["delivered_at"] != nil {
							tx.AddError(errors.New("simulated acknowledgement failure"))
						}
					}
				}))
				require.Error(t, model.DeliverTaskBillingLog(context.Background(), pending[0].ID, BuildTaskBillingDeliveryLog))
				require.NoError(t, model.DB.Callback().Update().Remove("test:delivery-ack-fail"))
				var count int64
				require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
				assert.EqualValues(t, 2, count)
			}
			assert.Equal(t, 920, getUserQuota(t, 9919), "funding already committed before log delivery")
			assert.True(t, model.HasTerminalTasksPendingBilling(), "settled funding must not hide an undelivered log")
			require.NoError(t, model.DeliverTaskBillingLog(context.Background(), pending[0].ID, BuildTaskBillingDeliveryLog))
			require.NoError(t, model.DeliverTaskBillingLog(context.Background(), pending[0].ID, BuildTaskBillingDeliveryLog))
			applied, _, err = model.ApplyTaskBillingTarget(task, 80)
			require.NoError(t, err)
			assert.False(t, applied)
			assert.Equal(t, 920, getUserQuota(t, 9919))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Order("id").Find(&logs).Error)
			require.Len(t, logs, 2)
			assert.Equal(t, 80, logs[1].CompletionTokens)
			var user model.User
			require.NoError(t, model.DB.First(&user, 9919).Error)
			assert.Equal(t, 80, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			var sum struct{ Quota, Count, Tokens int }
			require.NoError(t, model.DB.Model(&model.QuotaData{}).Select("SUM(quota) AS quota, SUM(count) AS count, SUM(token_used) AS tokens").Where("user_id = ?", 9919).Scan(&sum).Error)
			assert.Equal(t, 80, sum.Quota)
			assert.Equal(t, 1, sum.Count)
			assert.Equal(t, 80, sum.Tokens)
		})
	}
}

func TestTaskBillingDeliveryCancellationRollsBackAndRecovers(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%t", split), func(t *testing.T) {
			// Cancellation can discard SQLite's connection; use files so rollback and
			// subsequent recovery inspect the same durable database.
			oldDB, oldLogDB := model.DB, model.LOG_DB
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "main.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskBillingDelivery{}, &model.User{}, &model.Channel{}, &model.Log{}, &model.QuotaData{}))
			model.DB, model.LOG_DB = db, db
			if split {
				logs, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, logs.AutoMigrate(&model.Log{}))
				model.LOG_DB = logs
				t.Cleanup(func() { sqlDB, _ := logs.DB(); sqlDB.Close() })
			}
			t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB; sqlDB, _ := db.DB(); sqlDB.Close() })
			seedUser(t, 9920, 900)
			task := makeTask(9920, 0, 100, 0, BillingSourceWallet, 0)
			task.TaskID = model.GenerateTaskID()
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
			require.NoError(t, task.InsertWithContext(context.Background()))
			pending, err := model.PendingTaskBillingDeliveries(context.Background(), task.ID, 10)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callbackReached, bounded := false, false
			require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register("test:cancel-delivery", func(tx *gorm.DB) {
				if tx.Statement.Table != "logs" {
					return
				}
				callbackReached = true
				_, bounded = tx.Statement.Context.Deadline()
				cancel() // Cancel while the log operation owns the main transaction.
				select {
				case <-tx.Statement.Context.Done():
					tx.AddError(tx.Statement.Context.Err())
				default:
					tx.AddError(errors.New("log operation did not inherit cancellation"))
				}
			}))
			DeliverTaskBillingLogs(ctx, task.ID, 10)
			require.NoError(t, model.LOG_DB.Callback().Create().Remove("test:cancel-delivery"))
			require.True(t, callbackReached)
			require.True(t, bounded, "log DB operation must have a deadline")
			var event model.TaskBillingDelivery
			require.NoError(t, db.First(&event, pending[0].ID).Error)
			assert.Zero(t, event.DeliveredAt)
			assert.Positive(t, event.NextRetryAt, "cancellation must not prevent retry scheduling")
			var count int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
			assert.Zero(t, count)
			var user model.User
			require.NoError(t, db.First(&user, 9920).Error)
			assert.Zero(t, user.UsedQuota)
			assert.Zero(t, user.RequestCount)
			assert.Equal(t, 900, user.Quota)
			require.NoError(t, model.DeliverTaskBillingLog(context.Background(), event.ID, BuildTaskBillingDeliveryLog))
			require.NoError(t, model.DeliverTaskBillingLog(context.Background(), event.ID, BuildTaskBillingDeliveryLog))
			require.NoError(t, db.First(&user, 9920).Error)
			assert.Equal(t, 100, user.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			assert.Equal(t, 900, user.Quota)
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			_, err = model.PendingTaskBillingDeliveries(ctx, 0, 1)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, model.DeferTaskBillingDelivery(ctx, event.ID), context.Canceled)
		})
	}
}

func TestTaskBillingDeliveryFailedBatchYieldsToUnattemptedWork(t *testing.T) {
	truncate(t)
	seedUser(t, 9921, 900)
	var events []model.TaskBillingDelivery
	for i := 0; i < 3; i++ {
		task := makeTask(9921, 0, 100, 0, BillingSourceWallet, 0)
		task.TaskID = model.GenerateTaskID()
		task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
		require.NoError(t, task.InsertWithContext(context.Background()))
		pending, err := model.PendingTaskBillingDeliveries(context.Background(), task.ID, 1)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		// Place the initial batch before the logical next polling pass, without
		// sleeping or relying on the host's scheduling speed.
		require.NoError(t, model.DB.Model(&model.TaskBillingDelivery{}).Where("id = ?", pending[0].ID).Update("next_retry_at", common.GetTimestamp()-60).Error)
		events = append(events, pending[0])
	}
	for _, event := range events[:2] {
		require.Error(t, model.DeliverTaskBillingLog(context.Background(), event.ID, func(*model.Task, model.TaskBillingDelivery) (*model.Log, error) {
			return nil, errors.New("persistent projection failure")
		}))
		require.NoError(t, model.DeferTaskBillingDelivery(context.Background(), event.ID))
		// Simulate the next slow polling pass after the retry delay has expired.
		require.NoError(t, model.DB.Model(&model.TaskBillingDelivery{}).Where("id = ?", event.ID).Update("next_retry_at", common.GetTimestamp()-1).Error)
	}
	pending, err := model.PendingTaskBillingDeliveries(context.Background(), 0, 2)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, events[2].ID, pending[0].ID)
	DeliverTaskBillingLogs(context.Background(), 0, 1)
	var delivered model.TaskBillingDelivery
	require.NoError(t, model.DB.First(&delivered, events[2].ID).Error)
	assert.Positive(t, delivered.DeliveredAt)
	assert.Equal(t, 900, getUserQuota(t, 9921))
	// New arrivals must in turn yield to older due retries, rather than all
	// receiving a permanent zero timestamp ahead of the retry queue.
	newTask := makeTask(9921, 0, 100, 0, BillingSourceWallet, 0)
	newTask.TaskID = model.GenerateTaskID()
	newTask.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	require.NoError(t, newTask.InsertWithContext(context.Background()))
	pending, err = model.PendingTaskBillingDeliveries(context.Background(), 0, 2)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, events[0].ID, pending[0].ID)
	assert.Equal(t, events[1].ID, pending[1].ID)
}

func TestTaskBillingDeliveryPreservesOrderAndRejectsConflictingLog(t *testing.T) {
	truncate(t)
	seedUser(t, 9922, 900)
	task := makeTask(9922, 0, 100, 0, BillingSourceWallet, 0)
	task.TaskID = model.GenerateTaskID()
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	require.NoError(t, task.InsertWithContext(context.Background()))
	task.PrivateData.AsyncBilling.ActualTokens = 80
	task.PrivateData.AsyncBilling.ActualUsageReported = true
	task.PrivateData.AsyncBilling.Operation = "settle"
	require.NoError(t, model.DB.Save(task).Error)
	applied, _, err := model.ApplyTaskBillingTarget(task, 80)
	require.NoError(t, err)
	require.True(t, applied)
	events, err := model.PendingTaskBillingDeliveries(context.Background(), task.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.ErrorContains(t, model.DeliverTaskBillingLog(context.Background(), events[1].ID, BuildTaskBillingDeliveryLog), "earlier task log")
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DeliverTaskBillingLog(context.Background(), events[0].ID, BuildTaskBillingDeliveryLog))
	bad, err := BuildTaskBillingDeliveryLog(task, events[1])
	require.NoError(t, err)
	bad.RequestId = fmt.Sprintf("task-billing:%d:%s", task.ID, events[1].Event)
	bad.Quota++ // Same idempotency key, conflicting financial evidence.
	require.NoError(t, model.LOG_DB.Create(bad).Error)
	require.ErrorContains(t, model.DeliverTaskBillingLog(context.Background(), events[1].ID, BuildTaskBillingDeliveryLog), "conflicts with frozen event")
	var user model.User
	require.NoError(t, model.DB.First(&user, 9922).Error)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 920, user.Quota)
	var pending model.TaskBillingDelivery
	require.NoError(t, model.DB.First(&pending, events[1].ID).Error)
	assert.Zero(t, pending.DeliveredAt)
	require.NoError(t, model.LOG_DB.Delete(bad).Error) // Correct only the test's conflicting log.
	require.NoError(t, model.DeliverTaskBillingLog(context.Background(), events[1].ID, BuildTaskBillingDeliveryLog))
	require.NoError(t, model.DeliverTaskBillingLog(context.Background(), events[1].ID, BuildTaskBillingDeliveryLog))
	require.NoError(t, model.DB.First(&user, 9922).Error)
	assert.Equal(t, 80, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 920, user.Quota)
}

func TestTaskBillingDeliveryHonorsHTTPRequestCancellation(t *testing.T) {
	truncate(t)
	seedUser(t, 9923, 900)
	task := makeTask(9923, 0, 100, 0, BillingSourceWallet, 0)
	task.TaskID = model.GenerateTaskID()
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	require.NoError(t, task.InsertWithContext(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil).WithContext(ctx)
	LogTaskConsumption(c, nil, task)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
	assert.Zero(t, count)
	pending, err := model.PendingTaskBillingDeliveries(context.Background(), task.ID, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	DeliverTaskBillingLogs(context.Background(), task.ID, 1)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, 900, getUserQuota(t, 9923))
}
