package controller

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// taskFundGuaranteeHandler drives the video creation funds guarantee. Per the
// contract it must be independently wakeable: it does NOT depend on the
// provider polling switch (constant.UpdateTask), unfinished tasks, or the
// technical attempt reconcile query — only on actual fund debts.
type taskFundGuaranteeHandler struct{}

func (taskFundGuaranteeHandler) Type() string { return model.SystemTaskTypeTaskFundGuarantee }

func (taskFundGuaranteeHandler) Enabled() bool {
	now := model.GetDBTimestamp()
	attempts, err := model.GetTaskCreateAttemptFundDebts(now, 1)
	if err != nil || len(attempts) > 0 {
		return true
	}
	tasks, manual, err := model.PendingVideoRefunds(now, 1)
	return err != nil || len(tasks) > 0 || len(manual) > 0
}

func (taskFundGuaranteeHandler) Interval() time.Duration { return 15 * time.Second }

func (taskFundGuaranteeHandler) NewPayload() any { return nil }

func (taskFundGuaranteeHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := service.RunVideoRefunds(ctx)
	summary := service.RunTaskCreateFundGuaranteeOnce(ctx)
	if summary.ScanFailed || summary.Failed > 0 || summary.Blocked > 0 {
		err = errors.New("video fund guarantee has pending failures")
	}
	if err != nil {
		service.NotifyVideoFundFailure()
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}
