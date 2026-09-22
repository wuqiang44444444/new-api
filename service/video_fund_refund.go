package service

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const VerificationScopeVideoRefund = "video.funds.refund"

type VideoRefundContext struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	Version string `json:"version"`
	Note    string `json:"note"`
}

func ProcessVideoRefund(ctx context.Context, kind string, id int64) error {
	var err error
	if kind == "task" {
		err = model.CompleteVideoTaskRefund(id)
	} else {
		_, err = model.ReleaseTaskCreateAttemptHoldWarranty(id, model.TaskCreateAttemptReleaseWarrantyManual, 0)
	}
	if err != nil {
		if retryErr := model.DeferVideoRefund(kind, id, errors.Is(err, model.ErrTaskCreateAttemptFundBlocked)); retryErr != nil {
			logger.LogWarn(ctx, "video refund retry scheduling failed")
		}
		return err
	}
	if kind == "task" {
		DeliverTaskBillingLogs(ctx, id, 10)
	}
	return nil
}

func RunVideoRefunds(ctx context.Context) error {
	ts, as, err := model.PendingVideoRefunds(model.GetDBTimestamp(), 100)
	if err != nil {
		return err
	}
	var failures error
	for _, t := range ts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := ProcessVideoRefund(ctx, "task", t.ID); err != nil {
			failures = errors.Join(failures, err)
			logger.LogWarn(ctx, "video task refund remains pending")
		}
	}
	for _, a := range as {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := ProcessVideoRefund(ctx, "attempt", a.ID); err != nil {
			failures = errors.Join(failures, err)
			logger.LogWarn(ctx, "video attempt refund remains pending")
		}
	}
	return failures
}

// Native creation persists Task before its request-level billing finalization.
// The admin refund must not race that final adjustment or guess old provenance.
func MarkVideoTaskFundingReady(task *model.Task) {
	if !model.IsVideoFundTask(task) || model.IsLinkVideoTaskClientProtocol(task.ClientProtocol) {
		return
	}
	if err := model.MarkVideoTaskFundingReady(task.ID); err != nil {
		logger.LogWarn(context.Background(), "native video funding completion requires verification")
	}
}
