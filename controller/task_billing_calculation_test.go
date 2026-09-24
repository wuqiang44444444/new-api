package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskBillingCalculationOwnershipAndHistoricalEvidence(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "calculation.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sql, _ := db.DB(); _ = sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	task := &model.Task{TaskID: "task-calculation", UserId: 100, Quota: 70, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{}}}
	require.NoError(t, db.Create(task).Error)
	for _, tc := range []struct {
		name             string
		user, role, code int
	}{{"owner", 100, common.RoleCommonUser, 200}, {"other user", 101, common.RoleCommonUser, 404}, {"admin", 101, common.RoleAdminUser, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/task/:task_id/billing", func(c *gin.Context) { c.Set("id", tc.user); c.Set("role", tc.role); TaskBillingCalculation(c) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/task/task-calculation/billing", nil))
			assert.Equal(t, tc.code, w.Code)
			if tc.code == 200 {
				assert.Contains(t, w.Body.String(), `"evidence":"historical"`)
				assert.Contains(t, w.Body.String(), `"quota":70`)
				assert.NotContains(t, w.Body.String(), "private_data")
			}
		})
	}
}

func TestTaskBillingCalculationDistinguishesInitialAndSettlementEvents(t *testing.T) {
	c := billingexpr.NewCalculation().Finish(70)
	task := &model.Task{Quota: 70, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{SettlementCalculationVersion: 1, SettlementCalculation: c}}}
	v := buildTaskCalculationView(task)
	assert.Equal(t, "historical", v.InitialEvidence)
	assert.Equal(t, "complete", v.Evidence)
	task.PrivateData.BillingContext.SettlementCalculation = nil
	assert.Equal(t, "missing", buildTaskCalculationView(task).Evidence)
	task.PrivateData.BillingContext.SettlementCalculation = billingexpr.NewCalculation().Finish(69)
	assert.Equal(t, "inconsistent", buildTaskCalculationView(task).Evidence)
}

func TestTaskBillingCalculationFullRefundPreservesOriginalPrice(t *testing.T) {
	for _, waived := range []int{0, 30} {
		task := &model.Task{Quota: 0, VideoRefund: model.VideoRefund{VideoRefundState: "refunded", VideoRefundQuota: 70, VideoRefundWaivedQuota: waived}, PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{CalculationVersion: 1, CalculationSource: "settlement", Calculation: billingexpr.NewCalculation().Finish(70 + waived)}}}
		view := buildTaskCalculationView(task)
		assert.Equal(t, "refunded", view.State)
		assert.Equal(t, "complete", view.Evidence)
		assert.Equal(t, 70, *view.RefundedQuota)
		assert.Nil(t, view.TargetQuota)
		assert.Equal(t, 0, view.Quota)
	}
}

func TestNativeCalculationDoesNotTreatProviderCompletionAsSettlement(t *testing.T) {
	task := &model.Task{Quota: 70, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{CalculationVersion: 1, InitialCalculation: billingexpr.NewCalculation().Finish(70)}}}
	view := buildTaskCalculationView(task)
	assert.Equal(t, "unknown", view.State, "a failed funding adjustment can leave the initial charge on a completed task")
	assert.Equal(t, 70, view.Quota)
	task.PrivateData.BillingContext.SettlementCalculation = billingexpr.NewCalculation().Finish(70)
	assert.Equal(t, "settled", buildTaskCalculationView(task).State)
	task.PrivateData.BillingContext.SettlementCalculation = nil
	task.PrivateData.BillingContext.PerCallBilling = true
	assert.Equal(t, "settled", buildTaskCalculationView(task).State, "successful per-call tasks explicitly keep the frozen charge")
	task.Status = model.TaskStatusFailure
	assert.Equal(t, "unknown", buildTaskCalculationView(task).State, "provider failure alone does not prove a refund")
}

func TestBatchCalculationUsesCommittedFundsWhileLogDeliveryIsPending(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch-calculation.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sql, _ := db.DB(); _ = sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}, &model.BatchJob{}, &model.BatchJobLine{}, &model.TaskBillingDelivery{}))
	user := &model.User{Username: "batch-calculation-owner", Quota: 1000}
	require.NoError(t, db.Create(user).Error)
	task := &model.Task{TaskID: "batch-calculation", Platform: "azure_batch", UserId: user.Id, Quota: 100, PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}}}
	require.NoError(t, db.Create(task).Error)
	frozen, err := common.Marshal(model.BatchFrozenSnapshot{InitialCalculations: billingexpr.CalculationArchive{"a": billingexpr.NewCalculation().Finish(100)}})
	require.NoError(t, err)
	job := &model.BatchJob{Id: "batch-calculation", TaskRowId: task.ID, UserId: user.Id, SettleState: model.BatchSettlePending, FrozenSnapshot: model.BillingText(frozen)}
	require.NoError(t, db.Create(job).Error)
	calculation, err := common.Marshal(billingexpr.NewCalculation().Finish(80))
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.BatchJobLine{JobId: job.Id, CustomId: "a", FinalQuota: 80, CalculationVersion: 1, Calculation: model.BillingText(calculation)}).Error)
	r := gin.New()
	r.GET("/batch/:id/billing", func(c *gin.Context) { c.Set("id", user.Id); BatchLineCalculation(c) })
	read := func() taskCalculationView {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/batch/batch-calculation/billing?custom_id=a", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Data taskCalculationView `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		return response.Data
	}
	pending := read()
	assert.Equal(t, 100, pending.Quota)
	require.NotNil(t, pending.TargetQuota)
	assert.Equal(t, 80, *pending.TargetQuota)
	applied, _, err := model.ApplyTaskBillingTarget(task, 80)
	require.NoError(t, err)
	require.True(t, applied)
	settled := read()
	assert.Equal(t, "settled", settled.State)
	assert.Equal(t, 80, settled.Quota)
	assert.Nil(t, settled.TargetQuota)
}

func TestLegacyVideoFailedFundingKeepsPrechargeWithoutClaimingSettlement(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "native-funding.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sql, _ := db.DB(); _ = sql.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}))
	user := &model.User{Username: "native-calculation-owner", Quota: 0}
	require.NoError(t, db.Create(user).Error)
	task := &model.Task{TaskID: "native-calculation", UserId: user.Id, Quota: 70, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{CalculationVersion: 1, InitialCalculation: billingexpr.NewCalculation().Finish(70)}}}
	task.Action = constant.TaskActionTextToVideo
	require.NoError(t, db.Create(task).Error)
	task.PrivateData.BillingContext.SettlementCalculationVersion = 1
	task.PrivateData.BillingContext.SettlementCalculation = billingexpr.NewCalculation().Finish(100)
	_, _, err = model.ApplyTaskBillingTarget(task, 100)
	require.ErrorIs(t, err, model.ErrTaskBillingInsufficientFunding)
	saved, err := model.GetTaskById(task.ID)
	require.NoError(t, err)
	view := buildTaskCalculationView(saved)
	assert.Equal(t, "unknown", view.State)
	assert.Equal(t, 70, view.Quota)
	assert.Equal(t, "initial", view.Source)
	assert.Nil(t, view.Settlement)
}
