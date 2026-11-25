package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

const ArkPrefixCacheModelVersion = setting.ArkPrefixCacheModelVersion

type ArkPrefixCachePilot = setting.ArkPrefixCachePilot

func ArkCachePilot() *ArkPrefixCachePilot { return setting.ArkCachePilot() }

type arkPrefixCacheSettlement struct{}

func init() { RegisterSystemTaskHandler(arkPrefixCacheSettlement{}) }

func (arkPrefixCacheSettlement) Type() string            { return "ark_prefix_cache_settlement" }
func (arkPrefixCacheSettlement) Interval() time.Duration { return time.Minute }
func (arkPrefixCacheSettlement) NewPayload() any         { return nil }

func (arkPrefixCacheSettlement) Enabled() bool {
	if model.DB == nil {
		return false
	}
	if ArkCachePilot() != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return model.ArkCacheOutstanding(ctx)
}

func (arkPrefixCacheSettlement) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	now := common.GetTimestamp()
	err := runArkCacheWork(ctx, func(opCtx context.Context) error { return model.ReconcileStaleArkPrefixSends(opCtx, now) }, nil)
	rows, dueErr := model.ArkPrefixCacheDue(ctx, now)
	err = errors.Join(err, dueErr)
	for _, row := range rows {
		err = errors.Join(err, runArkCacheWork(ctx,
			func(opCtx context.Context) error { return model.SettleArkPrefixCache(opCtx, row.ID, now) },
			func(opCtx context.Context) error { return model.DeferArkPrefixCacheSettlement(opCtx, row.ID, now) }))
	}
	logs, logErr := model.ArkPrefixCacheLogsDue(ctx, now)
	err = errors.Join(err, logErr)
	for _, row := range logs {
		err = errors.Join(err, runArkCacheWork(ctx,
			func(opCtx context.Context) error { return model.DeliverArkPrefixCacheLog(opCtx, row.ID, now) },
			func(opCtx context.Context) error { return model.DeferArkPrefixCacheLog(opCtx, row.ID, now) }))
	}
	pending, pendingErr := model.ArkSessionInferenceDue(ctx, now)
	err = errors.Join(err, pendingErr)
	for _, row := range pending {
		err = errors.Join(err, runArkCacheWork(ctx,
			func(opCtx context.Context) error { return model.SettleArkSessionInference(opCtx, row.ID, now) },
			func(opCtx context.Context) error { return model.DeferArkSessionInference(opCtx, row.ID, now) }))
	}
	inferenceLogs, inferenceLogErr := model.ArkSessionLogsDue(ctx, now)
	err = errors.Join(err, inferenceLogErr)
	for _, row := range inferenceLogs {
		err = errors.Join(err, runArkCacheWork(ctx,
			func(opCtx context.Context) error { return model.DeliverArkSessionLog(opCtx, row.ID) },
			func(opCtx context.Context) error { return model.DeferArkSessionLog(opCtx, row.ID, now) }))
	}
	deletions, deleteErr := model.ArkCacheDeletionsDue(ctx, now)
	err = errors.Join(err, deleteErr)
	for _, row := range deletions {
		opCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err = errors.Join(err, reconcileArkCacheDelete(opCtx, row, now))
		cancel()
	}
	if err != nil {
		failSystemTask(task, runnerID, errors.New("Ark prefix cache settlement or delivery will be retried"))
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		common.SysError("failed to acknowledge the Ark prefix cache settlement task")
	}
}

func runArkCacheWork(ctx context.Context, run, deferRetry func(context.Context) error) error {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := run(opCtx)
	cancel()
	if err == nil || deferRetry == nil || ctx.Err() != nil {
		return err
	}
	retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return errors.Join(err, deferRetry(retryCtx))
}
