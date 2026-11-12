package common

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

func generateMessageID(sender string) string {
	_, domain, _ := strings.Cut(sender, "@")
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain)
}

func smtpTLSConfig(config SMTPMailConfig) *tls.Config {
	return &tls.Config{
		ServerName:         config.Server,
		InsecureSkipVerify: config.SkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

// SendEmail submits an HTML message with the default transport deadline.
func SendEmail(subject string, receiver string, content string) error {
	return SendEmailContext(context.Background(), subject, receiver, content)
}
