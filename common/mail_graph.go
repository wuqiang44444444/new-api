package common

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
	"sync/atomic"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

var graphMailHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
var graphMailTokenCache struct {
	sync.Mutex
	config GraphMailConfig
	token  *oauth2.Token
}

func graphMailToken(ctx context.Context, config GraphMailConfig, endpoint string) (*oauth2.Token, error) {
	graphMailTokenCache.Lock()
	cached := graphMailTokenCache.token
	matches := graphMailTokenCache.config == config
	graphMailTokenCache.Unlock()
	if matches && cached.Valid() {
		return cached, nil
	}
	client := clientcredentials.Config{
		ClientID: config.ClientID, ClientSecret: config.ClientSecret,
		TokenURL: endpoint, Scopes: []string{"https://graph.microsoft.com/.default"},
		AuthStyle: oauth2.AuthStyleInParams,
	}
	token, err := client.Token(context.WithValue(ctx, oauth2.HTTPClient, graphMailHTTPClient))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Microsoft Graph authentication failed")
	}
	graphMailTokenCache.Lock()
	graphMailTokenCache.config, graphMailTokenCache.token = config, token
	graphMailTokenCache.Unlock()
	return token, nil
}

func sendGraphMail(ctx context.Context, config GraphMailConfig, subject string, recipients []string, content string) error {
	tokenURL := "https://login.microsoftonline.com/" + config.TenantID + "/oauth2/v2.0/token"
	mailbox := config.MailboxID
	if mailbox == "" {
		mailbox = config.Sender
	}
	sendURL := "https://graph.microsoft.com/v1.0/users/" + url.PathEscape(mailbox) + "/sendMail"
	return submitGraphMail(ctx, config, tokenURL, sendURL, subject, recipients, content)
}

func submitGraphMail(ctx context.Context, config GraphMailConfig, tokenURL, sendURL, subject string, recipients []string, content string) error {
	token, err := graphMailToken(ctx, config, tokenURL)
	if err != nil {
		return err
	}
	to := make([]map[string]map[string]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, map[string]map[string]string{"emailAddress": {"address": recipient}})
	}
	body, err := Marshal(map[string]any{"message": map[string]any{
		"subject": subject, "from": map[string]any{"emailAddress": map[string]string{"address": config.Sender}}, "body": map[string]string{"contentType": "HTML", "content": content}, "toRecipients": to,
	}})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	token.SetAuthHeader(request)
	request.Header.Set("Content-Type", "application/json")
	var connected atomic.Bool
	request = request.WithContext(httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) { connected.Store(true) },
	}))
	observeMailSubmission(ctx)
	response, err := graphMailHTTPClient.Do(request)
	if err != nil {
		if connected.Load() {
			return ErrMailSubmissionUnknown
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Microsoft Graph mail submission failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusAccepted {
		if response.StatusCode == http.StatusUnauthorized {
			graphMailTokenCache.Lock()
			if graphMailTokenCache.token == token {
				graphMailTokenCache.token = nil
			}
			graphMailTokenCache.Unlock()
		}
		if response.StatusCode >= http.StatusInternalServerError || response.StatusCode == http.StatusRequestTimeout {
			return fmt.Errorf("%w: Microsoft Graph returned HTTP %d", ErrMailSubmissionUnknown, response.StatusCode)
		}
		return fmt.Errorf("Microsoft Graph mail submission returned HTTP %d", response.StatusCode)
	}
	return nil
}
