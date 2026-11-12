package common

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

const (
	MailTransportOption        = "MailTransportSecret"
	MailTransportSMTP          = "smtp"
	MailTransportGraph         = "graph"
	ShortMailSubmissionTimeout = 12 * time.Second
)

type MailTransportConfig struct {
	Mode  string           `json:"mode"`
	SMTP  *SMTPMailConfig  `json:"smtp,omitempty"`
	Graph *GraphMailConfig `json:"graph,omitempty"`
}

type SMTPMailConfig struct {
	Server     string `json:"server"`
	Port       int    `json:"port"`
	Account    string `json:"account"`
	From       string `json:"from"`
	Token      string `json:"token,omitempty"`
	SSL        bool   `json:"ssl"`
	StartTLS   bool   `json:"start_tls"`
	SkipVerify bool   `json:"skip_verify"`
	ForceLogin bool   `json:"force_login"`
}

type GraphMailConfig struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	MailboxID    string `json:"mailbox_id,omitempty"`
	Sender       string `json:"sender"`
	ClientSecret string `json:"client_secret,omitempty"`
}

var (
	// ErrMailSubmissionUnknown means a provider may have accepted the message.
	ErrMailSubmissionUnknown = errors.New("mail submission outcome is unknown")
	errMailNotConfigured     = errors.New("Mail delivery is not configured")
	// ErrMailBusy indicates all local transport slots are in use.
	ErrMailBusy   = errors.New("Mail delivery is busy")
	mailSendSlots = make(chan struct{}, 4)
)

// LoadMailTransport is wired to the primary database before serving requests.
// The default also supports standalone users of the common SMTP package.
var LoadMailTransport = func(context.Context) (MailTransportConfig, error) {
	return CurrentMailTransport(), nil
}

func CurrentMailTransport() MailTransportConfig {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	if raw, exists := OptionMap[MailTransportOption]; exists {
		var config MailTransportConfig
		if UnmarshalJsonStr(Interface2String(raw), &config) == nil {
			return config
		}
		return MailTransportConfig{}
	}
	return MailTransportConfig{Mode: MailTransportSMTP, SMTP: &SMTPMailConfig{
		Server: SMTPServer, Port: SMTPPort, Account: SMTPAccount, From: SMTPFrom, Token: SMTPToken,
		SSL: SMTPSSLEnabled, StartTLS: SMTPStartTLSEnabled, SkipVerify: SMTPInsecureSkipVerify, ForceLogin: SMTPForceAuthLogin,
	}}
}

func IsBareMailAddress(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && !strings.ContainsAny(value, "\r\n")
}

func (c SMTPMailConfig) sender() string {
	if c.From != "" {
		return c.From
	}
	return c.Account
}

func (c MailTransportConfig) Validate() error {
	switch c.Mode {
	case MailTransportSMTP:
		if c.SMTP == nil || c.Graph != nil || c.SMTP.Server == "" || strings.ContainsAny(c.SMTP.Server, "/?# \t\r\n") ||
			c.SMTP.Port < 1 || c.SMTP.Port > 65535 || !IsBareMailAddress(c.SMTP.sender()) {
			return errors.New("Configure a valid SMTP server, port and sender address")
		}
		// Some existing SMTP relays intentionally use no authentication.
	case MailTransportGraph:
		if c.Graph == nil || c.SMTP != nil || !IsBareMailAddress(c.Graph.Sender) ||
			Validate.Var(c.Graph.TenantID, "uuid") != nil || Validate.Var(c.Graph.ClientID, "uuid") != nil || c.Graph.ClientSecret == "" {
			return errors.New("Configure the Graph tenant ID, client ID, secret and sender address")
		}
		if c.Graph.MailboxID != "" && Validate.Var(c.Graph.MailboxID, "uuid") != nil {
			return errors.New("Enter a valid Graph mailbox object ID or leave it blank")
		}
	default:
		return errMailNotConfigured
	}
	return nil
}

func MailTransportReady() bool {
	return CurrentMailTransport().Validate() == nil
}

// SendEmailContext submits HTML mail through one configuration snapshot. Calls
// from different workers share a bounded, non-blocking transport budget.
func SendEmailContext(ctx context.Context, subject, receiver, content string) error {
	return sendEmailContext(ctx, subject, receiver, content, false)
}

// SendAccountEmail allows interactive account mail to wait briefly for capacity.
func SendAccountEmail(ctx context.Context, subject, receiver, content string) error {
	return sendEmailContext(ctx, subject, receiver, content, true)
}

func sendEmailContext(ctx context.Context, subject, receiver, content string, waitForSlot bool) error {
	receivers := strings.Split(receiver, ";")
	for i, address := range receivers {
		receivers[i] = strings.TrimSpace(address)
		if !IsBareMailAddress(receivers[i]) {
			return fmt.Errorf("invalid mail recipient")
		}
	}
	if err := acquireMailSendSlot(ctx, waitForSlot); err != nil {
		return err
	}
	defer func() { <-mailSendSlots }()

	ctx, cancel := context.WithTimeout(ctx, SMTPTotalTimeout)
	defer cancel()
	config, err := LoadMailTransport(ctx)
	if err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if config.Mode == MailTransportGraph {
		return sendGraphMail(ctx, *config.Graph, subject, receivers, content)
	}
	err = sendSMTPMail(ctx, *config.SMTP, subject, receivers, content)
	var rejected *textproto.Error
	if errors.As(err, &rejected) {
		return fmt.Errorf("SMTP rejected the submission with status %d", rejected.Code)
	}
	return err
}

func acquireMailSendSlot(ctx context.Context, wait bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mailSendSlots <- struct{}{}:
		return nil
	default:
	}
	if !wait {
		return ErrMailBusy
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case mailSendSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrMailBusy
	}
}
