package common

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
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

func newSMTPClient(ctx context.Context, addr string) (net.Conn, *smtp.Client, error) {
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
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		secure := tls.Client(conn, smtpTLSConfig())
		if err := secure.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, nil, err
		}
		transport = secure
	}
	client, err := smtp.NewClient(transport, SMTPServer)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if SMTPStartTLSEnabled && !SMTPSSLEnabled {
		supported, _ := client.Extension("STARTTLS")
		if !supported {
			client.Close()
			return nil, nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			client.Close()
			return nil, nil, err
		}
	}
	return transport, client, nil
}

// SendEmailContext 发送单封 HTML 邮件：DNS/连接/TLS/认证/DATA 全过程受
// SMTPTotalTimeout 与 ctx 约束，连接上设置绝对 deadline，取消真实生效。
func SendEmailContext(ctx context.Context, subject string, receiver string, content string) (sendErr error) {
	ctx, cancel := context.WithTimeout(ctx, SMTPTotalTimeout)
	defer cancel()
	accepted := false
	defer func() {
		if accepted {
			sendErr = nil
		} else if ctx.Err() != nil {
			sendErr = ctx.Err()
		} else if deadline, ok := ctx.Deadline(); sendErr != nil && ok && !time.Now().Before(deadline) {
			// The socket deadline can fire just before the context timer callback.
			sendErr = context.DeadlineExceeded
		}
	}()
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	id, err2 := generateMessageID()
	if err2 != nil {
		return err2
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, SystemName, SMTPFrom, encodedSubject, time.Now().Format(time.RFC1123Z), id, content))
	auth := getSMTPAuth()
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	to := strings.Split(receiver, ";")
	_, client, err := newSMTPClient(ctx, addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldAuthenticateSMTP() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, receiver := range to {
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
