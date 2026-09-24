package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBatchMissingResultLineDistinguishesCollectionAndCommittedRefund(t *testing.T) {
	for _, status := range []string{"cancelled", "expired", "failed"} {
		t.Run(status, func(t *testing.T) {
			oldDB := model.DB
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch.db")), &gorm.Config{})
			require.NoError(t, err)
			model.DB = db
			t.Cleanup(func() {
				model.DB = oldDB
				sql, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sql.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}, &model.BatchJob{}, &model.BatchJobLine{}, &model.TaskBillingDelivery{}))
			user := &model.User{Username: "batch-owner", Quota: 900}
			require.NoError(t, db.Create(user).Error)
			task := &model.Task{TaskID: "batch-projection", Platform: "azure_batch", UserId: user.Id, Quota: 100, PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}}}
			require.NoError(t, db.Create(task).Error)
			encoded, err := common.Marshal(model.BatchFrozenSnapshot{LineInputs: map[string]int{"omitted": 1}, InitialCalculations: billingexpr.CalculationArchive{"omitted": billingexpr.NewCalculation().Finish(100)}})
			require.NoError(t, err)
			job := &model.BatchJob{Id: task.TaskID, TaskRowId: task.ID, UserId: user.Id, PublicStatus: status, DeliveryState: model.BatchDeliveryProcessing, SettleState: model.BatchSettlePending, FrozenSnapshot: model.BillingText(encoded)}
			require.NoError(t, db.Create(job).Error)
			r := gin.New()
			r.GET("/batch/:id/calculation", func(c *gin.Context) { c.Set("id", user.Id); BatchLineCalculation(c) })
			read := func(customID string, expectedStatus int) taskCalculationView {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/batch/batch-projection/calculation?custom_id="+customID, nil))
				require.Equal(t, expectedStatus, w.Code)
				var response struct {
					Data taskCalculationView `json:"data"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				return response.Data
			}
			collecting := read("omitted", 200)
			assert.Equal(t, 100, collecting.Quota)
			assert.Equal(t, "initial", collecting.Source)
			assert.Nil(t, collecting.TargetQuota)
			assert.Nil(t, collecting.RefundedQuota)

			// Only the durable complete-result commit proves absence is uncharged.
			require.NoError(t, model.CommitBatchResultLines(job, nil))
			pending := read("omitted", 200)
			assert.Equal(t, "not_charged", pending.Source)
			assert.Equal(t, 100, pending.Quota)
			require.NotNil(t, pending.TargetQuota)
			assert.Zero(t, *pending.TargetQuota)
			assert.Nil(t, pending.RefundedQuota)
			assert.Nil(t, pending.Settlement, "do not manufacture a recorded calculation")

			applied, _, err := model.ApplyTaskBillingTarget(task, 0)
			require.NoError(t, err)
			require.True(t, applied)
			settled := read("omitted", 200)
			assert.Equal(t, "settled", settled.State, "funds can commit before job log delivery")
			assert.Zero(t, settled.Quota)
			assert.Equal(t, "complete", settled.Evidence)
			require.NotNil(t, settled.RefundedQuota)
			assert.Equal(t, 100, *settled.RefundedQuota)
			assert.Nil(t, settled.TargetQuota)
			read("unknown", 404)
			var count int64
			require.NoError(t, db.Model(&model.BatchJobLine{}).Count(&count).Error)
			assert.Zero(t, count, "projection must not invent provider rows")
			require.NoError(t, db.First(user, user.Id).Error)
			assert.Equal(t, 1000, user.Quota)
		})
	}
}

func TestBatchBillingPaginationIncludesMissingAndHistoricalInputRows(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch-pages.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sql, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sql.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.BatchJob{}, &model.BatchJobLine{}))
	frozen := model.BatchFrozenSnapshot{LineInputs: map[string]int{}}
	// 101 accepted IDs exercise the real 100-row dashboard boundary, including
	// historical inputs without recorded precharge calculations.
	for i := 0; i < 101; i++ {
		frozen.LineInputs[fmt.Sprintf("request-%03d", i)] = 1
	}
	encoded, err := common.Marshal(frozen)
	require.NoError(t, err)
	job := &model.BatchJob{Id: "batch-pages", UserId: 1, LineCount: 101, PublicStatus: "expired", DeliveryState: model.BatchDeliveryReady, FrozenSnapshot: model.BillingText(encoded)}
	require.NoError(t, db.Create(job).Error)
	require.NoError(t, db.Create(&model.BatchJobLine{JobId: job.Id, CustomId: "request-050", Status: "completed", InputTokens: 5, OutputTokens: 2, FinalQuota: 20}).Error)
	r := gin.New()
	r.GET("/batch/:id/billing", func(c *gin.Context) { c.Set("id", 1); BatchBillingDetails(c) })
	for _, offset := range []int{0, 100, 101} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/batch/batch-pages/billing?offset=%d", offset), nil))
		require.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Data struct {
				Lines   []model.BatchBillingLineView `json:"lines"`
				HasMore bool                         `json:"has_more"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, offset == 0, response.Data.HasMore)
		require.Len(t, response.Data.Lines, min(100, 101-offset))
		for i, row := range response.Data.Lines {
			assert.Equal(t, fmt.Sprintf("request-%03d", offset+i), row.CustomId)
			if row.CustomId == "request-050" {
				assert.Equal(t, "completed", row.Status)
				assert.Equal(t, 20, row.FinalQuota)
				assert.False(t, row.UsageUnavailable)
			} else {
				assert.Equal(t, "not_charged", row.Status)
				assert.Zero(t, row.FinalQuota)
				assert.True(t, row.UsageUnavailable)
				assert.False(t, row.Estimated)
			}
		}
	}
}

func TestBatchHistoricalResultWithoutCalculationKeepsRecordedCharge(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "historical-batch.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sql, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sql.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.BatchJob{}, &model.BatchJobLine{}))
	for _, hasInitial := range []bool{false, true} {
		t.Run(fmt.Sprint(hasInitial), func(t *testing.T) {
			id := fmt.Sprintf("historical-%v", hasInitial)
			task := &model.Task{TaskID: id, Platform: "azure_batch", UserId: 1, Quota: 80, PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStateSettled}}}
			require.NoError(t, db.Create(task).Error)
			frozen := model.BatchFrozenSnapshot{LineInputs: map[string]int{"a": 5}}
			if hasInitial {
				frozen.InitialCalculations = billingexpr.CalculationArchive{"a": billingexpr.NewCalculation().Finish(100)}
			}
			encoded, err := common.Marshal(frozen)
			require.NoError(t, err)
			job := &model.BatchJob{Id: id, TaskRowId: task.ID, UserId: 1, DeliveryState: model.BatchDeliveryReady, FrozenSnapshot: model.BillingText(encoded)}
			require.NoError(t, db.Create(job).Error)
			require.NoError(t, db.Create(&model.BatchJobLine{JobId: id, CustomId: "a", Status: "completed", FinalQuota: 80}).Error)
			r := gin.New()
			r.GET("/batch/:id/calculation", func(c *gin.Context) { c.Set("id", 1); BatchLineCalculation(c) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/batch/"+id+"/calculation?custom_id=a", nil))
			require.Equal(t, 200, w.Code)
			var response struct {
				Data taskCalculationView `json:"data"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, 80, response.Data.Quota)
			assert.Equal(t, "settlement", response.Data.Source)
			assert.Equal(t, "historical", response.Data.Evidence)
			assert.False(t, response.Data.ChargeUnknown)
			assert.Nil(t, response.Data.Settlement)
		})
	}
}
