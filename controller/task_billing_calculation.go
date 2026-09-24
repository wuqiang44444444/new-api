package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/gin-gonic/gin"
)

type taskCalculationView struct {
	ChargeUnknown   bool                     `json:"charge_unknown,omitempty"`
	RefundedQuota   *int                     `json:"refunded_quota,omitempty"`
	WaivedQuota     int                      `json:"waived_quota,omitempty"`
	Quota           int                      `json:"quota"`
	State           string                   `json:"state"`
	Source          string                   `json:"source"`
	Evidence        string                   `json:"evidence"`
	InitialEvidence string                   `json:"initial_evidence"`
	Initial         *billingexpr.Calculation `json:"initial,omitempty"`
	Settlement      *billingexpr.Calculation `json:"settlement,omitempty"`
	TargetQuota     *int                     `json:"target_quota,omitempty"`
}

func calculationEvidence(c *billingexpr.Calculation, version, quota int) string {
	if c == nil {
		if version > 0 {
			return "missing"
		}
		return "historical"
	}
	if c.Version != 1 || c.Quota != quota {
		return "inconsistent"
	}
	return "complete"
}

// This projection never invokes a pricing function or reads current prices.
func buildTaskCalculationView(task *model.Task) taskCalculationView {
	v := taskCalculationView{Quota: task.Quota, State: "pending", Source: "initial", Evidence: "historical", InitialEvidence: "historical"}
	initialVersion, settlementVersion := 0, 0
	if bc := task.PrivateData.BillingContext; bc != nil {
		v.Initial, v.Settlement = bc.InitialCalculation, bc.SettlementCalculation
		initialVersion = bc.CalculationVersion
		settlementVersion = bc.SettlementCalculationVersion
		if settlementVersion > 0 || v.Settlement != nil {
			settlementVersion = 1
			v.Source = "settlement"
		}
	}
	if task.Status.IsTerminal() {
		// Provider completion alone does not prove that a funding adjustment or
		// refund committed. Native tasks have no separate billing state machine.
		v.State = "unknown"
		if v.Settlement != nil && calculationEvidence(v.Settlement, settlementVersion, task.Quota) == "complete" {
			v.State = "settled"
		} else if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling && task.Status == model.TaskStatusSuccess {
			// Successful per-call tasks explicitly retain the accepted precharge.
			v.State = "settled"
		}
	}
	if a := task.PrivateData.AsyncBilling; a != nil {
		v.State = string(a.State)
		v.TargetQuota = a.TargetQuota
		if a.CalculationSource != "" {
			v.Source = a.CalculationSource
		}
		settlementVersion = a.CalculationVersion
		v.Settlement = a.Calculation
	}
	initialQuota := task.Quota
	if v.Initial != nil {
		initialQuota = v.Initial.Quota
	}
	v.InitialEvidence = calculationEvidence(v.Initial, initialVersion, initialQuota)
	if v.Source == "initial" {
		v.Evidence = calculationEvidence(v.Initial, initialVersion, task.Quota)
	} else {
		expected := task.Quota
		if v.TargetQuota != nil && v.State != "settled" {
			expected = *v.TargetQuota
		}
		v.Evidence = calculationEvidence(v.Settlement, settlementVersion, expected)
	}
	if task.VideoRefundState == "refunded" {
		v.State, v.TargetQuota = "refunded", nil
		v.RefundedQuota, v.WaivedQuota = &task.VideoRefundQuota, task.VideoRefundWaivedQuota
		if v.Source == "initial" {
			v.Evidence = calculationEvidence(v.Initial, initialVersion, task.VideoRefundQuota)
		} else {
			v.Evidence = calculationEvidence(v.Settlement, settlementVersion, task.VideoRefundQuota+task.VideoRefundWaivedQuota)
		}
	}
	return v
}

// Dashboard sessions can inspect their own applications, as on the task list.
func TaskBillingCalculation(c *gin.Context) {
	var task *model.Task
	var exists bool
	var err error
	if c.GetInt("role") >= common.RoleAdminUser {
		task, exists, err = model.GetByOnlyTaskId(c.Param("task_id"))
	} else {
		task, exists, err = model.GetByTaskId(c.GetInt("id"), c.Param("task_id"))
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Task billing is unavailable"})
		return
	}
	if !exists || task == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Task not found"})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": buildTaskCalculationView(task)})
}

func BatchLineCalculation(c *gin.Context) {
	job, err := model.GetDueBatchJobById(c.Param("id"))
	if err != nil || (job.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Batch job not found"})
		return
	}
	line, err := model.GetBatchLineCalculation(job, c.Query("custom_id"))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Batch billing is unavailable"})
		return
	}
	if line == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Batch line not found"})
		return
	}
	task, err := model.GetTaskById(job.TaskRowId)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Batch billing is unavailable"})
		return
	}
	// The funding transaction can commit before the Batch log is delivered.
	// Job state describes that delivery too; the Task owns committed funds.
	state := job.SettleState
	if a := task.PrivateData.AsyncBilling; a != nil && a.State == model.TaskBillingStateSettled {
		state = string(a.State)
	} else if state == model.BatchSettleSettled {
		state = "unknown"
	}
	initial, settlement, quota := line.Initial, line.Settlement, line.Quota
	v := taskCalculationView{Quota: quota, State: state, Source: "settlement", Initial: initial, Settlement: settlement}
	v.InitialEvidence = "historical"
	if initial != nil {
		v.InitialEvidence = calculationEvidence(initial, 1, initial.Quota)
	}
	v.Evidence = calculationEvidence(settlement, 0, quota)
	if !line.ResultPresent && !line.NotCharged {
		v.Source = "initial"
		v.Evidence = v.InitialEvidence
	}
	if line.NotCharged {
		v.Source, v.Evidence = "not_charged", "complete"
		if state == string(model.TaskBillingStateSettled) && initial != nil {
			refund := initial.Quota
			v.RefundedQuota = &refund
		}
	}
	if (line.ResultPresent || line.NotCharged) && v.State != string(model.TaskBillingStateSettled) {
		v.TargetQuota = &quota
		if initial != nil {
			v.Quota = initial.Quota
		} else {
			v.ChargeUnknown = true
		}
	}
	if !line.ResultPresent && !line.NotCharged && initial == nil {
		v.ChargeUnknown = true
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}
