package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var resourceAlertDeliveryQueued atomic.Bool

func recordResourceAlert(ctx context.Context, key, node string, observation resourceObservation, at int64) (bool, error) {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return false, err
	}
	var body strings.Builder
	writeOperationsMailRows(&body, [][2]string{
		{"资源 / Resource", observation.label},
		{"网关节点 / Gateway node", node},
		{"使用率 / Usage", fmt.Sprintf("%.1f%%", observation.usage)},
		{"阈值 / Threshold", fmt.Sprintf("%d%%", observation.threshold)},
		{"采样时间 / Sampled at (UTC+8)", time.Unix(at, 0).In(errorReportLocation()).Format("2006-01-02 15:04:05")},
	})
	title := "资源阈值告警 / Resource threshold alert"
	episode := model.ResourceAlertEpisode{
		OpenKey: &key, Resource: observation.key, OpenedAt: at,
		Subject:  title + " — " + observation.label,
		BodyHTML: renderOperationsMail(title, body.String()),
	}
	_, err = model.OpenResourceAlert(ctx, episode, recipients)
	return err == nil, err
}

func queueResourceAlertDeliveries() {
	if !resourceAlertDeliveryQueued.CompareAndSwap(false, true) {
		return
	}
	if !EnqueueMailWork(func() {
		defer resourceAlertDeliveryQueued.Store(false)
		if err := deliverResourceAlerts(time.Now, common.SendEmailContext); err != nil {
			common.SysError("resource alert delivery could not be processed")
		}
	}) {
		// The next sample can wake the worker. Pending rows remain in the database.
		resourceAlertDeliveryQueued.Store(false)
	}
}

func deliverResourceAlerts(now func() time.Time, send mailSendFunc) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := model.MarkExpiredResourceAlertDeliveriesUnknown(ctx, now().Unix(), 8)
	cancel()
	if err != nil {
		return err
	}
	owner := common.GetRandomString(24)
	for range 4 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		recipients, err := model.GetOperationsMailRecipients(ctx)
		if err != nil {
			cancel()
			return err
		}
		at := now().Unix()
		delivery, err := model.ClaimResourceAlertDelivery(ctx, owner, at, at+30)
		if err != nil || delivery == nil {
			cancel()
			return err
		}
		state, next := model.MailDeliveryStopped, int64(0)
		var sendErr error
		if slices.Contains(recipients, delivery.Recipient) {
			var episode model.ResourceAlertEpisode
			episode, sendErr = model.GetResourceAlertEpisode(ctx, delivery.EpisodeID)
			cancel()
			if sendErr == nil {
				sendCtx, cancelSend := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
				sendErr = send(sendCtx, episode.Subject, delivery.Recipient, episode.BodyHTML)
				cancelSend()
			}
			state = model.MailDeliveryAccepted
			switch {
			case errors.Is(sendErr, common.ErrMailSubmissionUnknown):
				state = model.MailDeliveryUnknown
			case sendErr != nil:
				state = model.MailDeliveryRetryWait
				next = now().Unix() + mailDeliveryBackoff(delivery.Attempts)
			}
		} else {
			cancel()
		}
		if sendErr != nil {
			common.SysError(fmt.Sprintf("resource alert delivery %d: %s", delivery.ID, state))
		}
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		err = model.FinishResourceAlertDelivery(ctx, delivery.ID, owner, state, next)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}
