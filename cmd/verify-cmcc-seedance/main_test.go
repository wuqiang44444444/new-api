package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestVideoCredentialVerificationAcceptsAbsoluteHTTPGateway(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "/proxy/api/v3/contents/generations/tasks", request.URL.Path)
		assert.Equal(t, "1", request.URL.Query().Get("page_size"))
		assert.Equal(t, "Bearer video-key", request.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"total":0,"items":[]}`)),
			Header:     make(http.Header),
		}, nil
	})}

	ok, err := verifyVideoCredential(context.Background(), client, "http://provider.example/proxy", "video-key")
	require.NoError(t, err)
	assert.True(t, ok)

	_, err = verifyVideoCredential(context.Background(), client, "ftp://provider.example", "video-key")
	require.ErrorContains(t, err, "video_origin_invalid")
}

func TestVideoListValidationAcceptsProviderEnvelopeWithoutExposingPrivateItems(t *testing.T) {
	payload := []byte(`{"total":1,"items":[{"id":"private-task-id","content":{"video_url":"https://signed.example/private"}}]}`)
	require.NoError(t, validateVideoListResponse(payload))

	report := verificationReport{ChannelID: 79, CustomerModel: expectedCustomerModel, VideoAuthenticated: true}
	encoded, err := common.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-task-id")
	assert.NotContains(t, string(encoded), "signed.example")
}

func TestVideoListValidationRejectsUntrustedShapes(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"total":-1,"items":[]}`),
		[]byte(`not-json`),
	} {
		require.Error(t, validateVideoListResponse(payload))
	}
}

func TestSafeVerificationErrorCodeDoesNotEchoArbitraryProviderText(t *testing.T) {
	assert.Equal(t, "video_http_401", safeVerificationErrorCode(errors.New("video_http_401")))
	assert.Equal(t, "verification_failed", safeVerificationErrorCode(errors.New("private provider response: credential rejected")))
}

func TestReadChannelCredentialsUsesCurrentSeedanceChannelType(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "cmcc-verification.db")
	database, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	_, err = database.Exec(`
		CREATE TABLE channels (
			id INTEGER PRIMARY KEY,
			status INTEGER NOT NULL,
			base_url TEXT NOT NULL,
			key TEXT NOT NULL,
			models TEXT NOT NULL,
			model_mapping TEXT NOT NULL,
			type INTEGER NOT NULL
		);
		CREATE TABLE channel_asset_credentials (
			channel_id INTEGER PRIMARY KEY,
			access_key_id TEXT NOT NULL,
			secret_access_key TEXT NOT NULL
		);
	`)
	require.NoError(t, err)
	_, err = database.Exec(
		`INSERT INTO channels (id, status, base_url, key, models, model_mapping, type) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		79,
		2,
		"https://provider.example",
		"video-key",
		expectedCustomerModel,
		`{"seedance-2.0-cmcc":"doubao-seedance-2.0"}`,
		constant.ChannelTypeSeedanceLink,
	)
	require.NoError(t, err)
	_, err = database.Exec(
		`INSERT INTO channel_asset_credentials (channel_id, access_key_id, secret_access_key) VALUES (?, ?, ?)`,
		79,
		"asset-access-key",
		"asset-secret-key",
	)
	require.NoError(t, err)

	credential, err := readChannelCredentials(databasePath, 79)
	require.NoError(t, err)
	assert.Equal(t, "doubao-seedance-2.0", credential.ProviderMapping)
}
