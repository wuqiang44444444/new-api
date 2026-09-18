package common

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendEmailContextCancelsBlockedSMTPStages(t *testing.T) {
	for _, stage := range []string{"greeting", "implicit_tls", "starttls", "data"} {
		t.Run(stage, func(t *testing.T) {
			withSMTPSettings(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			host, port, err := net.SplitHostPort(listener.Addr().String())
			require.NoError(t, err)
			SMTPServer = host
			SMTPPort, err = strconv.Atoi(port)
			require.NoError(t, err)
			SMTPAccount = ""
			SMTPToken = ""
			SMTPFrom = "sender@example.com"
			SMTPSSLEnabled = stage == "implicit_tls"
			SMTPStartTLSEnabled = stage == "starttls"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- SendEmailContext(ctx, "test", "ops@example.com", "body") }()
			conn, err := listener.Accept()
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
			rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
			if stage == "starttls" || stage == "data" {
				require.NoError(t, writeSMTPLine(rw, "220 test ESMTP"))
				line, err := rw.ReadString('\n')
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(line, "EHLO"))
				if stage == "starttls" {
					require.NoError(t, writeSMTPLine(rw, "250-test"))
					require.NoError(t, writeSMTPLine(rw, "250 STARTTLS"))
					line, err = rw.ReadString('\n')
					require.NoError(t, err)
					require.Equal(t, "STARTTLS\r\n", line)
					require.NoError(t, writeSMTPLine(rw, "220 start TLS"))
					// Consume a TLS handshake byte, then deliberately never finish TLS.
					_, err = rw.ReadByte()
					require.NoError(t, err)
				} else {
					require.NoError(t, writeSMTPLine(rw, "250 test"))
					for _, command := range []string{"MAIL FROM:", "RCPT TO:", "DATA"} {
						line, err = rw.ReadString('\n')
						require.NoError(t, err)
						require.True(t, strings.HasPrefix(line, command))
						response := "250 OK"
						if command == "DATA" {
							response = "354 send data"
						}
						require.NoError(t, writeSMTPLine(rw, response))
					}
					for {
						line, err = rw.ReadString('\n')
						require.NoError(t, err)
						if line == ".\r\n" {
							break
						}
					}
					// DATA acceptance is intentionally withheld.
				}
			}
			cancel()
			select {
			case err := <-done:
				assert.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("cancel did not interrupt SMTP")
			}
		})
	}
}

func TestSendEmailContextDeadlineCoversGreeting(t *testing.T) {
	withSMTPSettings(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	SMTPServer = "127.0.0.1"
	SMTPPort = listener.Addr().(*net.TCPAddr).Port
	SMTPFrom = "sender@example.com"
	SMTPAccount = ""
	SMTPToken = ""
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- SendEmailContext(ctx, "test", "ops@example.com", "body") }()
	conn, err := listener.Accept()
	require.NoError(t, err)
	defer conn.Close()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("deadline did not cover greeting")
	}
}

func TestSendEmailAcceptedDespiteQuitFailure(t *testing.T) {
	for _, cancelAfterAcceptance := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelAfterAcceptance), func(t *testing.T) {
			withSMTPSettings(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			SMTPServer = "127.0.0.1"
			SMTPPort = listener.Addr().(*net.TCPAddr).Port
			SMTPFrom, SMTPAccount, SMTPToken = "sender@example.com", "", ""
			SMTPSSLEnabled, SMTPStartTLSEnabled = false, false
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- SendEmailContext(ctx, "fixture", "receiver@example.com", "fixture") }()
			conn, err := listener.Accept()
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
			rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
			require.NoError(t, writeSMTPLine(rw, "220 test ESMTP"))
			for _, command := range []string{"EHLO", "MAIL FROM:", "RCPT TO:", "DATA"} {
				line, err := rw.ReadString('\n')
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(line, command))
				response := "250 OK"
				if command == "DATA" {
					response = "354 send data"
				}
				require.NoError(t, writeSMTPLine(rw, response))
			}
			for {
				line, err := rw.ReadString('\n')
				require.NoError(t, err)
				if line == ".\r\n" {
					break
				}
			}
			require.NoError(t, writeSMTPLine(rw, "250 message accepted"))
			// Receiving QUIT proves the client consumed DATA's acceptance before failure.
			line, err := rw.ReadString('\n')
			require.NoError(t, err)
			require.Equal(t, "QUIT\r\n", line)
			if cancelAfterAcceptance {
				cancel()
			}
			require.NoError(t, conn.Close())
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("SMTP did not finish")
			}
		})
	}
}
