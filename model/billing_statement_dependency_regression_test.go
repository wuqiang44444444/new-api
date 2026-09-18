package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementExternalEvidenceChanges(t *testing.T) {
	for _, tc := range []struct {
		name    string
		task    bool
		blocked bool
	}{
		{"missing_original_added", false, true},
		{"original_corrected", false, true},
		{"duplicate_refund_next_month", false, true},
		{"other_key_refund", false, false},
		{"other_customer_refund", false, false},
		{"missing_task_added", true, true},
		{"same_task_id_other_key", true, false},
		{"same_task_id_other_customer", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			enableVersionSwitch(t)
			require.NoError(t, db.AutoMigrate(&Task{}))
			ctx := context.Background()
			month := naturalMonthStartAt(1756656100)
			original := Log{Id: 900, UserId: 11, TokenId: 7, Type: LogTypeConsume, Quota: 10, CreatedAt: month - 100}
			if tc.name == "original_corrected" {
				require.NoError(t, db.Create(&original).Error)
			}
			refund := Log{UserId: 11, TokenId: 7, Type: LogTypeRefund, Quota: 10, CreatedAt: month + 100,
				Other: `{"admin_info":{"original_preauth_log_id":900}}`}
			if tc.task {
				refund.Other = `{"task_id":"missing-task","model_price":0}`
			}
			require.NoError(t, db.Create(&refund).Error)
			_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
			require.NoError(t, err)
			markDraftPendingWithVector(t, ctx, 11, month, draft)

			switch tc.name {
			case "missing_original_added":
				require.NoError(t, db.Create(&original).Error)
			case "original_corrected":
				require.NoError(t, db.Model(&original).Update("quota", 20).Error)
			case "duplicate_refund_next_month", "other_key_refund", "other_customer_refund":
				refund.Id = 0
				refund.CreatedAt = month + 32*86400
				if tc.name == "other_key_refund" {
					refund.TokenId = 8
				}
				if tc.name == "other_customer_refund" {
					refund.UserId = 12
				}
				require.NoError(t, db.Create(&refund).Error)
			default:
				task := Task{UserId: 11, AppID: 7, TaskID: "missing-task"}
				if tc.name == "same_task_id_other_key" {
					task.AppID = 8
				}
				if tc.name == "same_task_id_other_customer" {
					task.UserId = 12
				}
				require.NoError(t, db.Create(&task).Error)
			}
			_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, tc.name, "", "", 7)
			if tc.blocked {
				assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
				assert.False(t, committed)
			} else {
				require.NoError(t, err)
				assert.True(t, committed)
			}
		})
	}
}
