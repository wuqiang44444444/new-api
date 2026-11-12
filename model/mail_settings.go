package model

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var mailTransportUpdateMu sync.Mutex

func LoadMailTransport(ctx context.Context) (common.MailTransportConfig, error) {
	var saved Option
	tx := DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
	if err := tx.Where(&Option{Key: common.MailTransportOption}).Take(&saved).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return loadLegacySMTP(tx)
		}
		return common.MailTransportConfig{}, errors.New("could not read active mail configuration")
	}
	var config common.MailTransportConfig
	if err := common.UnmarshalJsonStr(saved.Value, &config); err != nil {
		return config, errors.New("active mail configuration is invalid")
	}
	return config, nil
}

func loadLegacySMTP(tx *gorm.DB) (common.MailTransportConfig, error) {
	var rows []Option
	if err := tx.Where(clause.IN{Column: clause.Column{Name: "key"}, Values: []any{
		"SMTPServer", "SMTPPort", "SMTPAccount", "SMTPFrom", "SMTPToken",
		"SMTPSSLEnabled", "SMTPStartTLSEnabled", "SMTPInsecureSkipVerify", "SMTPForceAuthLogin",
	}}).Find(&rows).Error; err != nil {
		return common.MailTransportConfig{}, errors.New("could not read legacy mail configuration")
	}
	smtp := common.SMTPMailConfig{
		Port:       587,
		StartTLS:   common.GetEnvOrDefaultBool("SMTP_STARTTLS_ENABLE", common.GetEnvOrDefaultBool("SMTP_STARTTLS_ENABLED", false)),
		SkipVerify: common.GetEnvOrDefaultBool("SMTP_INSECURE_SKIP_VERIFY", common.GetEnvOrDefaultBool("SMTP_TLS_INSECURE_SKIP_VERIFY", false)),
	}
	for _, row := range rows {
		switch row.Key {
		case "SMTPServer":
			smtp.Server = row.Value
		case "SMTPPort":
			smtp.Port, _ = strconv.Atoi(row.Value)
		case "SMTPAccount":
			smtp.Account = row.Value
		case "SMTPFrom":
			smtp.From = row.Value
		case "SMTPToken":
			smtp.Token = row.Value
		case "SMTPSSLEnabled":
			smtp.SSL = row.Value == "true"
		case "SMTPStartTLSEnabled":
			smtp.StartTLS = row.Value == "true"
		case "SMTPInsecureSkipVerify":
			smtp.SkipVerify = row.Value == "true"
		case "SMTPForceAuthLogin":
			smtp.ForceLogin = row.Value == "true"
		}
	}
	return common.MailTransportConfig{Mode: common.MailTransportSMTP, SMTP: &smtp}, nil
}

// UpdateMailTransport serializes credential preservation with the authoritative
// row. The one persisted value switches protocol and credentials atomically.
func UpdateMailTransport(ctx context.Context, config common.MailTransportConfig) error {
	mailTransportUpdateMu.Lock()
	defer mailTransportUpdateMu.Unlock()

	var encoded []byte
	err := DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		placeholder := Option{Key: common.MailTransportOption}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&placeholder).Error; err != nil {
			return err
		}
		var saved Option
		if err := lockForUpdate(tx).Where(&Option{Key: common.MailTransportOption}).First(&saved).Error; err != nil {
			return err
		}
		var current common.MailTransportConfig
		if saved.Value == "" {
			var err error
			current, err = loadLegacySMTP(tx)
			if err != nil {
				return err
			}
		} else {
			if err := common.UnmarshalJsonStr(saved.Value, &current); err != nil {
				return err
			}
		}
		if smtp, previous := config.SMTP, current.SMTP; smtp != nil && previous != nil &&
			smtp.Token == "" && smtp.Server == previous.Server && smtp.Account == previous.Account {
			smtp.Token = previous.Token
		}
		if graph, previous := config.Graph, current.Graph; graph != nil && previous != nil &&
			graph.ClientSecret == "" && graph.TenantID == previous.TenantID && graph.ClientID == previous.ClientID {
			graph.ClientSecret = previous.ClientSecret
		}
		if err := config.Validate(); err != nil {
			return common.NewMessage(err.Error())
		}
		var err error
		encoded, err = common.Marshal(config)
		if err != nil {
			return err
		}
		return tx.Model(&saved).Update("value", string(encoded)).Error
	})
	if err != nil {
		return err
	}
	return updateOptionMap(common.MailTransportOption, string(encoded))
}
