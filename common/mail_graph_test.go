package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mailGraphTestTransport func(*http.Request) (*http.Response, error)

func (f mailGraphTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGraphMailSelectedTransportAndAcceptance(t *testing.T) {
	var tokens atomic.Int32
	var mail atomic.Int32
	var responseStatus atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/00000000-0000-4000-8000-000000000001/oauth2/v2.0/token":
			tokens.Add(1)
			assert.NoError(t, request.ParseForm())
			assert.Equal(t, "client_credentials", request.Form.Get("grant_type"))
			assert.Equal(t, "https://graph.microsoft.com/.default", request.Form.Get("scope"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fixture","token_type":"Bearer","expires_in":3600}`))
		case "/v1.0/users/sender@example.com/sendMail", "/v1.0/users/00000000-0000-4000-8000-000000000003/sendMail":
			mail.Add(1)
			assert.Equal(t, "Bearer fixture", request.Header.Get("Authorization"))
			var body struct {
				Message struct {
					Subject      string
					From         struct{ EmailAddress struct{ Address string } }
					Body         struct{ ContentType, Content string }
					ToRecipients []struct{ EmailAddress struct{ Address string } }
				}
			}
			if assert.NoError(t, DecodeJson(request.Body, &body)) && assert.Len(t, body.Message.ToRecipients, 1) {
				assert.Equal(t, "receiver@example.com", body.Message.ToRecipients[0].EmailAddress.Address)
				assert.Equal(t, "HTML", body.Message.Body.ContentType)
				assert.Equal(t, "Fixture", body.Message.Subject)
				assert.Equal(t, "sender@example.com", body.Message.From.EmailAddress.Address)
			}
			if status := responseStatus.Load(); status == -1 {
				conn, _, err := w.(http.Hijacker).Hijack()
				if assert.NoError(t, err) {
					_ = conn.Close()
				}
				return
			} else if status != 0 {
				w.WriteHeader(int(status))
				_, _ = w.Write([]byte("private provider error"))
				return
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	originalClient, originalOptions := graphMailHTTPClient, OptionMap
	t.Cleanup(func() { graphMailHTTPClient, OptionMap = originalClient, originalOptions })
	graphMailHTTPClient = &http.Client{Transport: mailGraphTestTransport(func(request *http.Request) (*http.Response, error) {
		copy := request.Clone(request.Context())
		copy.URL.Scheme, copy.URL.Host = "http", server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(copy)
	})}
	config := MailTransportConfig{Mode: MailTransportGraph, Graph: &GraphMailConfig{
		TenantID: "00000000-0000-4000-8000-000000000001", ClientID: "00000000-0000-4000-8000-000000000002", Sender: "sender@example.com", ClientSecret: "test-secret",
	}}
	raw, err := Marshal(config)
	require.NoError(t, err)
	OptionMap = map[string]string{MailTransportOption: string(raw)}
	for range 2 {
		require.NoError(t, SendEmailContext(context.Background(), "Fixture", "receiver@example.com", "<p>fixture</p>"))
	}
	assert.EqualValues(t, 1, tokens.Load())
	assert.EqualValues(t, 2, mail.Load())
	config.Graph.MailboxID = "00000000-0000-4000-8000-000000000003"
	raw, err = Marshal(config)
	require.NoError(t, err)
	OptionMap[MailTransportOption] = string(raw)
	responseStatus.Store(http.StatusForbidden)
	err = SendEmailContext(context.Background(), "Fixture", "receiver@example.com", "<p>fixture</p>")
	require.ErrorContains(t, err, "403")
	assert.NotContains(t, err.Error(), "private provider error")
	assert.EqualValues(t, 3, mail.Load())
	responseStatus.Store(http.StatusTooManyRequests)
	err = SendEmailContext(context.Background(), "Fixture", "receiver@example.com", "<p>fixture</p>")
	require.ErrorContains(t, err, "429")
	assert.NotErrorIs(t, err, ErrMailSubmissionUnknown)
	for _, status := range []int32{http.StatusBadGateway, -1} {
		responseStatus.Store(status)
		err := SendEmailContext(context.Background(), "Fixture", "receiver@example.com", "<p>fixture</p>")
		assert.ErrorIs(t, err, ErrMailSubmissionUnknown)
	}
	config.SMTP = &SMTPMailConfig{}
	assert.Error(t, config.Validate())
}
