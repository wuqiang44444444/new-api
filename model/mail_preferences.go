package model

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type MailType string

const (
	MailChannelBalance       MailType = "channel_upstream_balance"
	MailLowUpstreamBalance   MailType = "low_upstream_balance"
	MailChannelService       MailType = "channel_service"
	MailChannelConnection    MailType = "channel_connection"
	MailChannelRateLimit     MailType = "channel_rate_limit"
	MailChannelCredential    MailType = "channel_credential"
	MailChannelConfiguration MailType = "channel_configuration"
	MailChannelDisabled      MailType = "channel_disabled"
	MailChannelRecovered     MailType = "channel_recovered"
	MailChannelTest          MailType = "channel_test"
	MailUpstreamModels       MailType = "upstream_models"
	MailCPU                  MailType = "resource_cpu"
	MailMemory               MailType = "resource_memory"
	MailDisk                 MailType = "resource_disk"
	MailEvidenceCleanup      MailType = "evidence_cleanup"
	MailVideoFunds           MailType = "video_funds"
	MailWalletRisk           MailType = "wallet_risk"
	MailDailyErrors          MailType = "daily_errors"
	MailPlatformUsage        MailType = "platform_usage"
	MailCustomerStatus       MailType = "customer_status"
	MailPersonalUsage        MailType = "personal_usage"
	MailWalletSummary        MailType = "wallet_summary"
	MailWelcome              MailType = "account_created"
	MailTokenCreated         MailType = "token_created"
	MailWalletLow            MailType = "wallet_low"
	MailSubscriptionLow      MailType = "subscription_low"
	MailWalletReceipt        MailType = "wallet_receipt"
	MailAccountSecurity      MailType = "account_security"
)

type MailTypeDefinition struct {
	ID        MailType `json:"id"`
	Group     string   `json:"group"`
	Label     string   `json:"label"`
	Recipient string   `json:"recipient"`
}

var mailTypes = []MailTypeDefinition{
	{MailChannelBalance, "channels", "Upstream payment rejection", "operations"},
	{MailLowUpstreamBalance, "channels", "Upstream low balance", "operations"},
	{MailChannelService, "channels", "Upstream service failure", "operations"},
	{MailChannelConnection, "channels", "Channel connection failure", "operations"},
	{MailChannelRateLimit, "channels", "Persistent channel rate limiting", "operations"},
	{MailChannelCredential, "channels", "Inactive channel credentials", "operations"},
	{MailChannelConfiguration, "channels", "Channel configuration failure", "operations"},
	{MailChannelDisabled, "channels", "Channel automatically disabled", "root"},
	{MailChannelRecovered, "channels", "Channel automatically recovered", "root"},
	{MailChannelTest, "channels", "Bulk channel test completed", "root"},
	{MailUpstreamModels, "channels", "Upstream model changes and check failures", "subscribers"},
	{MailCPU, "alerts", "CPU threshold alert", "operations"},
	{MailMemory, "alerts", "Memory threshold alert", "operations"},
	{MailDisk, "alerts", "Disk threshold alert", "operations"},
	{MailEvidenceCleanup, "alerts", "Evidence cleanup failure", "operations"},
	{MailVideoFunds, "alerts", "Video refund or hold release failure", "root"},
	{MailWalletRisk, "alerts", "High-risk wallet change", "finance"},
	{MailDailyErrors, "reports", "Daily system error report", "operations"},
	{MailPlatformUsage, "reports", "Daily platform usage report", "operations"},
	{MailCustomerStatus, "reports", "Daily customer status report", "operations"},
	{MailPersonalUsage, "reports", "Daily personal usage report", "account"},
	{MailWalletSummary, "reports", "Daily manual wallet operations report", "finance"},
	{MailWelcome, "users", "Welcome email", "personal"},
	{MailTokenCreated, "users", "API key created", "personal"},
	{MailWalletLow, "users", "Low wallet balance", "personal"},
	{MailSubscriptionLow, "users", "Low subscription quota", "personal"},
	{MailWalletReceipt, "users", "Manual wallet change or top-up receipt", "account"},
	{MailAccountSecurity, "users", "Account security changes", "account"},
}

const mailPreferencesOption = "mail_preferences"
const financeMailRecipientsOption = "finance_mail_recipients"
const legacyWelcomeOption = "notification_setting.send_email_on_account_created"
const legacyTokenCreatedOption = "notification_setting.send_email_on_token_created"
const MailStopPreferenceDisabled = "preference_disabled"

var ErrInvalidMailPreferences = errors.New("invalid mail preferences")

type MailPreference struct {
	Enabled   bool  `json:"enabled"`
	ChangedAt int64 `json:"changed_at,omitempty"` // Unix nanoseconds; suppress work queued before a switch changed.
}

type MailPreferences struct {
	Types map[MailType]MailPreference `json:"types"`
	// Preserve the old lifecycle switches' effect on non-email notifications.
	LegacyLifecycleChannels map[MailType]bool `json:"legacy_lifecycle_channels,omitempty"`
}

func MailTypeCatalog() []MailTypeDefinition { return append([]MailTypeDefinition(nil), mailTypes...) }

func (p MailPreferences) Enabled(kind MailType) bool { return p.Types[kind].Enabled }
func (p MailPreferences) Allows(kind MailType, queuedAt time.Time) bool {
	pref := p.Types[kind]
	return pref.Enabled && (queuedAt.IsZero() || queuedAt.UnixNano() > pref.ChangedAt)
}

func LoadMailPreferences(ctx context.Context) (MailPreferences, error) {
	return loadMailPreferences(DB.WithContext(ctx), false)
}

func loadMailPreferences(tx *gorm.DB, lock bool) (MailPreferences, error) {
	if lock {
		tx = lockForUpdate(tx)
	}
	var row Option
	result := tx.Where(&Option{Key: mailPreferencesOption}).Find(&row)
	if result.Error != nil {
		return MailPreferences{}, result.Error
	}
	prefs := MailPreferences{Types: make(map[MailType]MailPreference, len(mailTypes))}
	for _, kind := range mailTypes {
		prefs.Types[kind.ID] = MailPreference{Enabled: kind.ID != MailWelcome && kind.ID != MailTokenCreated}
	}
	if row.Value != "" {
		var saved MailPreferences
		if err := common.UnmarshalJsonStr(row.Value, &saved); err != nil || saved.Types == nil {
			return MailPreferences{}, ErrInvalidMailPreferences
		}
		for kind, pref := range saved.Types {
			prefs.Types[kind] = pref
		}
		prefs.LegacyLifecycleChannels = saved.LegacyLifecycleChannels
		return prefs, nil
	}
	prefs.Types[MailPersonalUsage] = MailPreference{Enabled: common.GetEnvOrDefaultBool("PERSONAL_USAGE_REPORT_MAIL_ENABLED", true)}
	var legacy []Option
	keys := map[string]MailType{legacyWelcomeOption: MailWelcome, legacyTokenCreatedOption: MailTokenCreated}
	if err := tx.Session(&gorm.Session{NewDB: true}).Where(clause.IN{Column: clause.Column{Name: "key"}, Values: []any{
		legacyWelcomeOption, legacyTokenCreatedOption,
	}}).Find(&legacy).Error; err != nil {
		return MailPreferences{}, err
	}
	prefs.LegacyLifecycleChannels = make(map[MailType]bool, len(legacy))
	for _, opt := range legacy {
		enabled, _ := strconv.ParseBool(opt.Value)
		kind := keys[opt.Key]
		prefs.Types[kind] = MailPreference{Enabled: enabled}
		prefs.LegacyLifecycleChannels[kind] = enabled
	}
	return prefs, nil
}

func MigrateMailPreferences() error {
	_, err := saveMailPreferences(context.Background(), nil, nil)
	return err
}

func UpdateMailPreferences(ctx context.Context, changes map[MailType]bool, finance *string) (MailPreferences, error) {
	if len(changes) == 0 && finance == nil {
		return MailPreferences{}, ErrInvalidMailPreferences
	}
	for kind := range changes {
		known := false
		for _, entry := range mailTypes {
			if entry.ID == kind {
				known = true
				break
			}
		}
		if !known {
			return MailPreferences{}, ErrInvalidMailPreferences
		}
	}
	var recipients *string
	if finance != nil {
		addresses, err := operation_setting.ValidateErrorReportRecipients(*finance)
		if err != nil {
			return MailPreferences{}, ErrInvalidMailPreferences
		}
		normalized := strings.Join(addresses, ", ")
		recipients = &normalized
	}
	return saveMailPreferences(ctx, changes, recipients)
}

func saveMailPreferences(ctx context.Context, changes map[MailType]bool, finance *string) (MailPreferences, error) {
	var prefs MailPreferences
	err := DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		row := Option{Key: mailPreferencesOption}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return result.Error
		}
		if len(changes) == 0 && finance == nil {
			if result.RowsAffected == 0 {
				return nil
			}
			if addresses, err := operation_setting.ValidateErrorReportRecipients(os.Getenv("FINANCE_MAIL_RECIPIENTS")); err == nil {
				normalized := strings.Join(addresses, ", ")
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: financeMailRecipientsOption, Value: normalized}).Error; err != nil {
					return err
				}
			}
		}
		var err error
		prefs, err = loadMailPreferences(tx, true)
		if err != nil {
			return err
		}
		now := time.Now().UnixNano()
		for kind, enabled := range changes {
			old := prefs.Types[kind]
			if old.Enabled == enabled {
				continue
			}
			old.Enabled, old.ChangedAt = enabled, now
			if enabled {
				if err := queueOpenIncidentMail(tx, kind, now); err != nil {
					return err
				}
			} else if err := stopPendingMail(tx, kind); err != nil {
				return err
			}
			prefs.Types[kind] = old
		}
		if finance != nil {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&Option{Key: financeMailRecipientsOption, Value: *finance}).Error; err != nil {
				return err
			}
		}
		raw, err := common.Marshal(prefs)
		if err != nil {
			return err
		}
		if err := tx.Model(&Option{}).Where(&Option{Key: mailPreferencesOption}).Update("value", string(raw)).Error; err != nil {
			return err
		}
		return tx.Where(clause.IN{Column: clause.Column{Name: "key"}, Values: []any{legacyWelcomeOption, legacyTokenCreatedOption}}).Delete(&Option{}).Error
	})
	return prefs, err
}

// Suppress pending rows immediately, including future retries. A worker
// already holding a lease checks the same policy before transport submission.
func stopPendingMail(tx *gorm.DB, kind MailType) error {
	statuses := []string{MailDeliveryPending, MailDeliveryRetryWait}
	fields := map[string]any{"status": MailDeliveryStopped, "stop_reason": MailStopPreferenceDisabled, "lease_owner": "", "lease_until": 0}
	if kind == MailDailyErrors {
		fields["lease_until"] = nil
		return tx.Model(&ErrorReportDelivery{}).Where("status IN ?", statuses).Updates(fields).Error
	}
	if kinds := channelMailKinds(kind); len(kinds) > 0 {
		ids := tx.Model(&ChannelAlertEpisode{}).Select("id").Where("kind IN ?", kinds)
		return tx.Model(&ChannelAlertDelivery{}).Where("status IN ? AND episode_id IN (?)", statuses, ids).Updates(fields).Error
	}
	if resources := resourceMailKinds(kind); len(resources) > 0 {
		ids := tx.Model(&ResourceAlertEpisode{}).Select("id").Where("resource IN ?", resources)
		return tx.Model(&ResourceAlertDelivery{}).Where("status IN ? AND episode_id IN (?)", statuses, ids).Updates(fields).Error
	}
	return nil
}

var channelMailTypes = map[string]MailType{
	"upstream_balance": MailChannelBalance, "upstream_service": MailChannelService, "connection": MailChannelConnection,
	"rate_limited": MailChannelRateLimit, "credential": MailChannelCredential, "configuration": MailChannelConfiguration,
}
var resourceMailTypes = map[string]MailType{
	"cpu": MailCPU, "memory": MailMemory, "disk_data": MailDisk, "disk_log": MailDisk,
	EvidenceCleanupAlertResource: MailEvidenceCleanup, WalletRiskMail: MailWalletRisk, WalletReceiptMail: MailWalletReceipt,
	WalletSummaryMail: MailWalletSummary, PersonalUsageReportMail: MailPersonalUsage, PlatformUsageReportMail: MailPlatformUsage,
	CustomerStatusReportMail: MailCustomerStatus,
}

func ChannelAlertMailType(kind string) MailType { return channelMailTypes[kind] }
func ResourceMailType(resource string) MailType { return resourceMailTypes[resource] }

func channelMailKinds(kind MailType) []string {
	var keys []string
	for key, id := range channelMailTypes {
		if id == kind {
			keys = append(keys, key)
		}
	}
	return keys
}
func resourceMailKinds(kind MailType) []string {
	var keys []string
	for key, id := range resourceMailTypes {
		if id == kind {
			keys = append(keys, key)
		}
	}
	return keys
}

func isManagedMailOption(key string) bool {
	return key == mailPreferencesOption || key == financeMailRecipientsOption || strings.HasPrefix(key, "notification_setting.send_email_on_")
}

func MailPreferencesInTransaction(tx *gorm.DB) (MailPreferences, error) {
	return loadMailPreferences(tx, true)
}

func QueuedMailTime(nanos, seconds int64) time.Time {
	if nanos > 0 {
		return time.Unix(0, nanos)
	}
	return time.Unix(seconds, 0)
}

// Only open incidents that have never queued a message are eligible on enable.
func queueOpenIncidentMail(tx *gorm.DB, kind MailType, now int64) error {
	channelKinds := channelMailKinds(kind)
	var resources []string
	switch kind {
	case MailCPU, MailMemory, MailDisk, MailEvidenceCleanup:
		resources = resourceMailKinds(kind)
	}
	if len(channelKinds) == 0 && len(resources) == 0 {
		return nil
	}
	recipients, err := operationsMailRecipients(tx)
	if err != nil || len(recipients) == 0 {
		return err
	}
	if len(channelKinds) > 0 {
		var ids []int64
		existing := tx.Model(&ChannelAlertDelivery{}).Select("episode_id")
		if err := tx.Model(&ChannelAlertEpisode{}).Where("open_key IS NOT NULL AND closed_at = 0 AND (preview = ? OR preview IS NULL) AND kind IN ? AND id NOT IN (?)", false, channelKinds, existing).Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := queueChannelAlertMail(tx, id, recipients, now+1); err != nil {
				return err
			}
		}
		return nil
	}
	var ids []int64
	existing := tx.Model(&ResourceAlertDelivery{}).Select("episode_id")
	if err := tx.Model(&ResourceAlertEpisode{}).Where("open_key IS NOT NULL AND closed_at = 0 AND resource IN ? AND id NOT IN (?)", resources, existing).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := queueResourceAlertMail(tx, id, recipients, now+1); err != nil {
			return err
		}
	}
	return nil
}
