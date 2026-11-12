package common

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

const (
	SMTPDialTimeout  = 15 * time.Second
	SMTPTotalTimeout = 60 * time.Second
)

// smtpContextConn releases the cancellation callback when SMTP closes the connection.
// The deadline and cancellation are installed before greeting, EHLO or TLS I/O.
type smtpContextConn struct {
	net.Conn
	stop   func() bool
	cancel context.CancelFunc
}

func (c *smtpContextConn) Close() error {
	c.stop()
	c.cancel()
	return c.Conn.Close()
}

func newSMTPClient(ctx context.Context, addr string, config SMTPMailConfig) (net.Conn, *smtp.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, SMTPTotalTimeout)
	raw, err := (&net.Dialer{Timeout: SMTPDialTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	deadline, _ := ctx.Deadline()
	if err := raw.SetDeadline(deadline); err != nil {
		raw.Close()
		cancel()
		return nil, nil, err
	}
	conn := &smtpContextConn{Conn: raw, cancel: cancel}
	conn.stop = context.AfterFunc(ctx, func() { _ = raw.Close() })
	var transport net.Conn = conn
	if config.SSL || (config.Port == 465 && !config.StartTLS) {
		secure := tls.Client(conn, smtpTLSConfig(config))
		if err := secure.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, nil, err
		}
		transport = secure
	}
	client, err := smtp.NewClient(transport, config.Server)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if config.StartTLS && !config.SSL {
		supported, _ := client.Extension("STARTTLS")
		if !supported {
			client.Close()
			return nil, nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig(config)); err != nil {
			client.Close()
			return nil, nil, err
		}
	}
	return transport, client, nil
}

func sendSMTPMail(ctx context.Context, config SMTPMailConfig, subject string, receivers []string, content string) (sendErr error) {
	accepted := false
	awaitingReply := false
	defer func() {
		var rejection *textproto.Error
		if errors.As(sendErr, &rejection) {
			return
		}
		if accepted {
			sendErr = nil
		} else if ctx.Err() != nil {
			sendErr = ctx.Err()
		} else if deadline, ok := ctx.Deadline(); sendErr != nil && ok && !time.Now().Before(deadline) {
			sendErr = context.DeadlineExceeded
		}
		if awaitingReply && sendErr != nil {
			sendErr = fmt.Errorf("%w: %w", ErrMailSubmissionUnknown, sendErr)
		}
	}()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sender := config.sender()
	id := generateMessageID(sender)
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		strings.Join(receivers, ", "), strings.NewReplacer("\r", "", "\n", "").Replace(SystemName), sender, encodedSubject, time.Now().Format(time.RFC1123Z), id, content))
	addr := net.JoinHostPort(config.Server, fmt.Sprint(config.Port))
	_, client, err := newSMTPClient(ctx, addr, config)
	if err != nil {
		return err
	}
	defer client.Close()
	if config.Account != "" && config.Token != "" {
		if err = client.Auth(newSMTPAuth(config)); err != nil {
			return err
		}
	}
	observeMailSubmission(ctx)
	if err = client.Mail(sender); err != nil {
		return err
	}
	for _, receiver := range receivers {
		if err = client.Rcpt(receiver); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(mail); err != nil {
		return err
	}
	awaitingReply = true
	if err = w.Close(); err != nil {
		return err
	}
	// DATA's 250 reply is the delivery boundary. Cancellation or a broken QUIT
	// handshake after acceptance must not schedule the same message again.
	accepted = true
	if ctx.Err() == nil {
		_ = client.Quit()
	}
	return nil
}
