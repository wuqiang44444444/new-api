package main

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupHeldCorrection(t *testing.T) (*gorm.DB, *holdCorrectionPlan) {
	t.Helper()
	db, _ := setupTaskLogRepair(t)
	require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("id>0").Update("billing_source", "wallet").Error)
	require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeConsume, Quota: 0, CreatedAt: 1050, Other: `{"task_id":"public-task","large":9007199254740993,"admin_info":{"task_billing_state":"debt"}}`}).Error)
	plan, err := auditHeldTaskProjection(db)
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	require.EqualValues(t, 100, plan.Candidates[0].Quota)
	return db, plan
}

func TestHeldProjectionCorrectionIsLogOnlyAndIdempotent(t *testing.T) {
	db, plan := setupHeldCorrection(t)
	var beforeUser model.User
	var beforeToken model.Token
	var beforeTask model.Task
	var beforeAttempt model.TaskCreateAttempt
	require.NoError(t, db.First(&beforeUser, 9).Error)
	require.NoError(t, db.First(&beforeToken, 4).Error)
	require.NoError(t, db.First(&beforeTask).Error)
	require.NoError(t, db.First(&beforeAttempt).Error)
	for _, want := range []int{1, 0} {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			n, err := applyHeldTaskProjection(tx, plan, 2, 1, "reviewed correction")
			assert.Equal(t, want, n)
			return err
		}))
	}
	var got model.Log
	require.NoError(t, db.First(&got, plan.Candidates[0].LogID).Error)
	assert.Equal(t, 100, got.Quota)
	assert.Contains(t, got.Other, `9007199254740993`)
	assert.Contains(t, got.Other, `held_projection_correction`)
	var user model.User
	var token model.Token
	var task model.Task
	var attempt model.TaskCreateAttempt
	require.NoError(t, db.First(&user, 9).Error)
	require.NoError(t, db.First(&token, 4).Error)
	require.NoError(t, db.First(&task).Error)
	require.NoError(t, db.First(&attempt).Error)
	assert.Equal(t, beforeUser, user)
	assert.Equal(t, beforeToken, token)
	assert.Equal(t, beforeTask, task)
	assert.Equal(t, beforeAttempt, attempt)
}

func TestHeldProjectionRejectsChangedOrAmbiguousEvidence(t *testing.T) {
	for _, change := range []string{"late refund", "conflicting refund reference", "hold changed", "duplicate attempt", "conflicting attempt identity", "identity conflict", "altered plan", "wrong generation"} {
		t.Run(change, func(t *testing.T) {
			db, plan := setupHeldCorrection(t)
			gen := int64(2)
			switch change {
			case "late refund":
				require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, ChannelId: 8, ModelName: "video", Type: model.LogTypeRefund, Quota: 1, Other: `{"task_id":"public-task"}`}).Error)
			case "conflicting refund reference":
				require.NoError(t, db.Create(&model.Log{UserId: 9, TokenId: 4, Type: model.LogTypeRefund, Quota: 0, Other: fmt.Sprintf(`{"task_id":"another-task","admin_info":{"original_preauth_log_id":%d}}`, plan.Candidates[0].LogID)}).Error)
			case "hold changed":
				require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("id>0").Update("held_quota", 101).Error)
			case "duplicate attempt", "conflicting attempt identity":
				var a model.TaskCreateAttempt
				require.NoError(t, db.First(&a).Error)
				a.ID = 0
				a.AttemptID = "duplicate"
				if change == "conflicting attempt identity" {
					a.UserID = 99
				}
				require.NoError(t, db.Create(&a).Error)
			case "identity conflict":
				require.NoError(t, db.Model(&model.Log{}).Where("id=?", plan.Candidates[0].LogID).Update("token_id", 99).Error)
			case "altered plan":
				plan.Candidates[0].Quota++
			case "wrong generation":
				gen++
			}
			var before []model.Log
			require.NoError(t, db.Order("id").Find(&before).Error)
			err := db.Transaction(func(tx *gorm.DB) error { _, err := applyHeldTaskProjection(tx, plan, gen, 1, "reviewed"); return err })
			require.Error(t, err)
			var after []model.Log
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestHeldProjectionPlanFailureRollsBackEarlierCorrections(t *testing.T) {
	db, plan := setupHeldCorrection(t)
	bad := plan.Candidates[0]
	bad.TaskID = 999
	plan.Candidates = append(plan.Candidates, bad)
	require.Error(t, db.Transaction(func(tx *gorm.DB) error { _, err := applyHeldTaskProjection(tx, plan, 2, 1, "reviewed"); return err }))
	var log model.Log
	require.NoError(t, db.First(&log, plan.Candidates[0].LogID).Error)
	assert.Zero(t, log.Quota)
	assert.NotContains(t, log.Other, "held_projection_correction")
}

func TestHeldProjectionAcceptsNullOptionalAdminMetadata(t *testing.T) {
	db, plan := setupHeldCorrection(t)
	require.NoError(t, db.Model(&model.Log{}).Where("id=?", plan.Candidates[0].LogID).Update("other", `{"task_id":"public-task","admin_info":null}`).Error)
	plan, err := auditHeldTaskProjection(db)
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	for _, want := range []int{1, 0} {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			n, err := applyHeldTaskProjection(tx, plan, 2, 1, "reviewed correction")
			assert.Equal(t, want, n)
			return err
		}))
	}
}
