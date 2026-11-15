package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

func enqueueWalletRejectionNotice(userID int) {
	if userID > 0 {
		enqueueWalletNotice(userID, true, nil)
	}
}

// For legacy batched settlement, the supplied balance includes the pending
// local deduction. Other modes read the current wallet balance on the worker.
func enqueueWalletNotice(userID int, rejection bool, pendingBalance *int) {
	queuedAt := time.Now()
	if !quotaMailWork.enqueue(func() {
		logDroppedQuotaMail()
		ctx, cancel := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
		defer cancel()
		queued, err := queueWalletNotice(ctx, userID, queuedAt, rejection, pendingBalance)
		if err != nil {
			common.SysError("could not prepare personal wallet email")
		}
		if queued {
			queueResourceAlertDeliveries()
		}
	}) {
		quotaMailDropped.Add(1)
	}
}

func walletNoticeEmail(account *model.User, setting dto.UserSetting) string {
	if account.Status != common.UserStatusEnabled || (setting.NotifyType != "" && setting.NotifyType != dto.NotifyTypeEmail) {
		return ""
	}
	if setting.NotificationEmail != "" {
		return setting.NotificationEmail
	}
	return account.Email
}

func queueWalletNotice(ctx context.Context, userID int, at time.Time, rejection bool, pendingBalance *int) (bool, error) {
	prefs, err := model.LoadMailPreferences(ctx)
	if err != nil || !prefs.Allows(model.MailWalletLow, at) {
		return false, err
	}
	db := model.DB.WithContext(ctx)
	var account model.User
	if err := db.Select("id", "status", "email", "quota", "setting", "username", "display_name").First(&account, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	setting := account.GetSetting()
	address := walletNoticeEmail(&account, setting)
	if !common.IsBareMailAddress(address) {
		return false, nil
	}
	threshold := common.EffectiveQuotaWarningThreshold(setting.QuotaWarningThreshold)
	remaining := account.Quota
	if pendingBalance != nil {
		remaining = *pendingBalance
	} else if !rejection {
		remaining, err = walletNoticeBalance(account)
		if err != nil {
			return false, err
		}
	}
	if !rejection && remaining >= threshold {
		return false, nil
	}

	// A genuinely low balance is one episode until recovery. A request
	// that costs more than a positive balance above the warning threshold
	// gets a separate notice, at most once per Beijing day.
	identity := fmt.Sprintf("%s:%d", model.WalletLowNoticeMail, userID)
	var closedAt int64
	if remaining >= threshold {
		identity += ":" + at.In(errorReportLocation()).Format(time.DateOnly)
		closedAt = at.Unix()
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	var existing int64
	if err := db.Model(&model.ResourceAlertEpisode{}).Where("open_key = ?", key).Count(&existing).Error; err != nil || existing > 0 {
		return false, err
	}
	// The two reminders share a cooldown. Observed recovery rearms the
	// threshold reminder even when the previous episode started recently.
	recent := db.Model(&model.ResourceAlertEpisode{}).
		Where("resource = ? AND mail_user_id = ? AND opened_at > ?", model.WalletLowNoticeMail, userID, at.Add(-24*time.Hour).Unix())
	if !rejection {
		recent = recent.Where("open_key IS NOT NULL")
	}
	if err := recent.Count(&existing).Error; err != nil || existing > 0 {
		return false, err
	}

	titleKey, bodyKey := i18n.MsgNotifyWalletLowTitle, i18n.MsgNotifyWalletLowEmail
	if rejection {
		titleKey, bodyKey = i18n.MsgNotifyWalletRejectedTitle, i18n.MsgNotifyWalletRejectedEmail
	}
	title := i18n.Translate(setting.Language, titleKey)
	var body strings.Builder
	body.WriteString("<p>" + html.EscapeString(i18n.Translate(setting.Language, bodyKey, map[string]any{
		"Quota": formatWalletMailQuota(remaining), "Threshold": formatWalletMailQuota(threshold),
	})) + "</p>")
	writeOperationsMailRows(&body, [][2]string{
		{"账户 / Account", mailAccountLabel(userID, model.MailAccount{DisplayName: account.DisplayName, Username: account.Username}, "")},
		{"提醒时间（北京时间） / Reminder time (Beijing time)", at.In(errorReportLocation()).Format("2006-01-02 15:04:05")},
	})
	if link := consoleMailLink("/wallet"); link != "" {
		writeReportMailLink(&body, link, i18n.Translate(setting.Language, i18n.MsgNotifyWalletOpen))
	}
	return model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: model.WalletLowNoticeMail, MailUserID: userID,
		OpenedAt: at.Unix(), ClosedAt: closedAt, WalletRejection: rejection,
		Subject: title, BodyHTML: RenderMailTemplate(title, body.String()),
	}, []string{address})
}

func renderSubscriptionQuotaMail(remaining, threshold string, at time.Time) string {
	var body strings.Builder
	writeOperationsMailRows(&body, [][2]string{
		{"剩余订阅额度 / Remaining subscription quota", remaining},
		{"提醒阈值 / Warning threshold", threshold},
		{"提醒时间（北京时间） / Reminder time (Beijing time)", at.In(errorReportLocation()).Format("2006-01-02 15:04:05")},
	})
	body.WriteString("<p>请在钱包页核对当前订阅的剩余额度、重置时间和计费偏好。<br>Review your active subscription balance, reset schedule, and billing preference in the wallet.</p>")
	if link := consoleMailLink("/wallet"); link != "" {
		writeReportMailLink(&body, link, "查看订阅与额度 / Review subscription quota")
	}
	return body.String()
}

func formatWalletMailQuota(quota int) string {
	value := logger.FormatQuota(quota)
	if dot := strings.LastIndexByte(value, '.'); dot >= 0 && len(value)-dot == 7 {
		return value[:dot+3] + strings.TrimRight(value[dot+3:], "0")
	}
	return value
}

func walletNoticeBalance(account model.User) (int, error) {
	if common.BatchUpdateEnabled && common.RedisEnabled {
		return model.GetUserQuota(account.Id, false)
	}
	return account.Quota, nil
}

func (h *walletNoticeHandler) recoverWalletNotices(ctx context.Context, now time.Time) error {
	db := model.DB.WithContext(ctx)
	var notices []model.ResourceAlertEpisode
	if err := db.Select("id", "mail_user_id").
		Where("resource = ? AND closed_at = 0 AND open_key IS NOT NULL AND id > ?", model.WalletLowNoticeMail, h.recoveryCursor.Load()).
		Order("id").Limit(200).Find(&notices).Error; err != nil {
		return err
	}
	if len(notices) == 0 {
		h.recoveryCursor.Store(0)
		return nil
	}
	ids := make([]int, 0, len(notices))
	for _, notice := range notices {
		ids = append(ids, notice.MailUserID)
	}
	var accounts []model.User
	if err := db.Select("id", "status", "quota", "setting").Find(&accounts, ids).Error; err != nil {
		return err
	}
	byID := make(map[int]model.User, len(accounts))
	for _, account := range accounts {
		byID[account.Id] = account
	}
	for _, notice := range notices {
		account := byID[notice.MailUserID]
		threshold := common.EffectiveQuotaWarningThreshold(account.GetSetting().QuotaWarningThreshold)
		recovered := account.Status != common.UserStatusEnabled
		if !recovered {
			balance, err := walletNoticeBalance(account)
			if err != nil {
				return err
			}
			recovered = balance >= threshold
		}
		if recovered {
			if err := model.CloseWalletNotice(ctx, notice.ID, now.Unix()); err != nil {
				return err
			}
		}
		h.recoveryCursor.Store(notice.ID)
	}
	if len(notices) < 200 {
		h.recoveryCursor.Store(0)
	}
	return nil
}
