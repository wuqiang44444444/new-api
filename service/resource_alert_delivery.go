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
	"gorm.io/gorm"
)

var resourceAlertDeliveryQueued atomic.Bool

func resourceMailRecipientAllowed(ctx context.Context, episode model.ResourceAlertEpisode, address string) (bool, error) {
	switch episode.Resource {
	case model.WalletLowNoticeMail:
		if episode.OpenKey == nil && !episode.WalletRejection {
			return false, nil
		}
		var account model.User
		err := model.DB.WithContext(ctx).Select("id", "status", "email", "quota", "setting").First(&account, episode.MailUserID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		setting := account.GetSetting()
		if walletNoticeEmail(&account, setting) != address {
			return false, nil
		}
		// A dated episode with ClosedAt set also identifies legacy rejection-only notices.
		if !episode.WalletRejection && episode.ClosedAt == 0 {
			balance, err := walletNoticeBalance(account)
			if err != nil {
				return false, err
			}
			if balance >= common.EffectiveQuotaWarningThreshold(setting.QuotaWarningThreshold) {
				return false, model.CloseWalletNotice(ctx, episode.ID, time.Now().Unix())
			}
		}
		return true, nil
	case model.PersonalUsageReportMail, model.WalletReceiptMail:
		db := model.DB.WithContext(ctx)
		if episode.Resource == model.PersonalUsageReportMail {
			db = db.Where("status = ?", common.UserStatusEnabled)
		}
		email, err := model.MailUserEmail(db, episode.MailUserID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return email == address && common.IsBareMailAddress(email), err
	case model.WalletRiskMail, model.WalletSummaryMail:
		recipients, err := model.FinanceMailRecipients(model.DB.WithContext(ctx))
		return slices.Contains(recipients, address), err
	default:
		recipients, err := model.GetOperationsMailRecipients(ctx)
		return slices.Contains(recipients, address), err
	}
}

func recordResourceAlert(ctx context.Context, key, node string, observation resourceObservation, at int64) (bool, error) {

	var body strings.Builder
	writeOperationsMailRows(&body, [][2]string{
		{"资源 / Resource", observation.label},
		{"网关节点 / Gateway node", node},
		{"使用率 / Usage", fmt.Sprintf("%.1f%%", observation.usage)},
		{"阈值 / Threshold", fmt.Sprintf("%d%%", observation.threshold)},
		{"采样时间 / Sampled at (UTC+8)", time.Unix(at, 0).In(errorReportLocation()).Format("2006-01-02 15:04:05")},
	})
	body.WriteString("<p>检查节点上该资源的近期使用趋势和并发负载。<br>Review recent utilization and concurrent load on this node.</p>")
	body.WriteString("<p>系统信息仅限 Root。交接时请提供节点、读数和采样时间。<br>System Information requires Root access. Include the node, reading and sample time when escalating.</p>")
	if link := consoleMailLink("/system-info"); link != "" {
		writeReportMailLink(&body, link, "打开系统信息（Root） / Open system information (Root)")
	}
	title := "资源阈值告警 / Resource threshold alert"
	episode := model.ResourceAlertEpisode{
		OpenKey: &key, Resource: observation.key, OpenedAt: at,
		Subject:  title + " — " + observation.label,
		BodyHTML: RenderMailTemplate(title, body.String()),
	}
	_, err := model.OpenResourceAlert(ctx, episode, nil)
	return err == nil, err
}

func queueResourceAlertDeliveries() {
	if !resourceAlertDeliveryQueued.CompareAndSwap(false, true) {
		return
	}
	if !EnqueueMailWork(func() {
		defer resourceAlertDeliveryQueued.Store(false)
		if err := deliverResourceAlerts(time.Now, common.SendEmailContext); err != nil {
			common.SysError(fmt.Sprintf("resource alert delivery could not be processed: %v", err))
		}
	}) {
		// The next sample can wake the worker. Pending rows remain in the database.
		resourceAlertDeliveryQueued.Store(false)
	}
}

func deliverResourceAlerts(now func() time.Time, send mailSendFunc) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := model.RetryIncidentMail(ctx, false); err != nil {
		common.SysError("resource alert mail preparation retry failed")
	}
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	err := model.MarkExpiredResourceAlertDeliveriesUnknown(ctx, now().Unix(), 8)
	cancel()
	if err != nil {
		return err
	}
	owner := common.GetRandomString(24)
	for range 4 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		at := now().Unix()
		delivery, err := model.ClaimResourceAlertDelivery(ctx, owner, at, at+30)
		if err != nil || delivery == nil {
			cancel()
			return err
		}
		state, next, stopReason := model.MailDeliveryStopped, int64(0), ""
		episode, sendErr := model.GetResourceAlertEpisode(ctx, delivery.EpisodeID)
		var allowed bool
		if sendErr == nil {
			switch episode.Resource {
			case model.PersonalUsageReportMail, model.PlatformUsageReportMail, model.CustomerStatusReportMail:
				current, err := model.UsageReportMailVersionCurrent(ctx, episode)
				sendErr = err
				if err == nil && !current {
					// Preserve the original outbox row and its deduplication key for cutover reconciliation.
					state, next, stopReason = model.MailDeliveryRetryWait, at+int64((24*time.Hour).Seconds()), "report_version_unavailable"
				}
			}
			if sendErr == nil && stopReason == "" {
				allowed, sendErr = resourceMailRecipientAllowed(ctx, episode, delivery.Recipient)
			}
		}
		cancel()
		if allowed || sendErr != nil {
			if sendErr == nil {
				sendCtx, cancelSend := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
				sendErr = submitManagedMail(sendCtx, send, model.ResourceMailType(episode.Resource), model.QueuedMailTime(delivery.QueuedAt, episode.OpenedAt), episode.Subject, delivery.Recipient, episode.BodyHTML)
				cancelSend()
			}
			state = model.MailDeliveryAccepted
			switch {
			case errors.Is(sendErr, errMailPreferenceDisabled):
				state, stopReason = model.MailDeliveryStopped, model.MailStopPreferenceDisabled
				sendErr = nil
			case errors.Is(sendErr, common.ErrMailSubmissionUnknown):
				state = model.MailDeliveryUnknown
			case errors.Is(sendErr, common.ErrMailPermanentlyRejected):
				state = model.MailDeliveryStopped
			case sendErr != nil:
				state = model.MailDeliveryRetryWait
				next = now().Unix() + mailDeliveryBackoff(delivery.Attempts)
			}
		}
		if sendErr != nil {
			common.SysError(fmt.Sprintf("resource alert delivery %d: %s", delivery.ID, state))
		}
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		err = model.FinishResourceAlertDelivery(ctx, delivery.ID, owner, state, next, stopReason)
		cancel()
		if err != nil {
			return err
		}
		if stopReason == "report_version_unavailable" && delivery.StopReason != stopReason {
			common.SysError(fmt.Sprintf("usage report mail delivery %d held for cutover review (%s)", delivery.ID, stopReason))
		}
	}
	return nil
}
