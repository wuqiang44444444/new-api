package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Explicit opt-in only. The configuration database is opened read-only. Tests
// use synthetic content, never change options, and never print credentials.
func TestLocalInfrastructureAcceptance(t *testing.T) {
	path := os.Getenv("LOCAL_ACCEPTANCE_DB_PATH")
	if os.Getenv("RUN_LOCAL_ACCEPTANCE") != "1" || path == "" {
		t.Skip("requires explicit local infrastructure acceptance opt-in")
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.True(t, err == nil, "open read-only configuration database")
	conn, err := db.DB()
	require.NoError(t, err)
	defer conn.Close()
	var options []model.Option
	require.True(t, db.Where("key IN ?", []string{"ObjectStorageSetting", "SMTPServer", "SMTPFrom", "SMTPPort", "SMTPAccount", "SMTPToken", "SMTPSSLEnabled", "SMTPStartTLSEnabled", "SMTPInsecureSkipVerify", "SMTPForceAuthLogin"}).Find(&options).Error == nil, "read local infrastructure options")
	values := map[string]string{}
	for _, option := range options {
		values[option.Key] = option.Value
	}
	t.Run("ObjectStorage", func(t *testing.T) {
		var config system_setting.ObjectStorageConfig
		require.True(t, common.UnmarshalJsonStr(values[system_setting.ObjectStorageSettingOptionKey], &config) == nil, "decode object storage configuration")
		result := RunObjectStorageConnectionTest(config, config.Credential)
		for _, step := range result.Steps {
			require.True(t, step.Success, "object storage step: %s", step.Name)
		}
		require.False(t, result.CleanupFailed, "remove only this test's object")
		require.True(t, result.Success, "object storage acceptance")
	})
	t.Run("ExportFileRoundTrip", func(t *testing.T) {
		var config system_setting.ObjectStorageConfig
		require.True(t, common.UnmarshalJsonStr(values[system_setting.ObjectStorageSettingOptionKey], &config) == nil, "decode object storage configuration")
		var artifactStore TaskArtifactStore
		var err error
		switch config.Backend {
		case system_setting.ObjectStorageBackendAzureBlob:
			artifactStore, err = NewAzureBlobArtifactStore(config, config.Credential)
		case system_setting.ObjectStorageBackendS3:
			artifactStore, err = NewS3ArtifactStore(legacyS3Config(config, config.Credential))
		default:
			t.Fatal("local object store backend is not configured")
		}
		require.True(t, err == nil, "build configured export store")
		store, ok := artifactStore.(exportObjectStore)
		require.True(t, ok)
		key := "object-storage-connectivity-check/export-" + common.GetUUID() + ".csv"
		// A fresh random namespace is owned exclusively by this test, including
		// an ambiguous PUT response. Always attempt its cleanup independently.
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.True(t, store.ExportDeleteObject(ctx, key) == nil, "delete synthetic export object")
			require.True(t, artifactStore.(objectStorageProbeAdapter).probeHeadMissing(ctx, key) == nil, "confirm synthetic export object is absent")
		})
		payload := bytes.Repeat([]byte("synthetic-request,10,验收 fixture\n"), 40000)
		path := filepath.Join(t.TempDir(), "acceptance.csv")
		require.NoError(t, os.WriteFile(path, payload, 0600))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		require.True(t, store.ExportPutFile(ctx, key, "text/csv; charset=utf-8", path, int64(len(payload))) == nil, "upload complete synthetic export file")
		signed, _, err := store.ExportPresignURL(key, time.Minute, "验收-fixture.csv")
		require.True(t, err == nil, "sign export download with UTF-8 filename")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, signed, nil)
		require.True(t, err == nil, "build signed download request")
		client := &http.Client{Timeout: 150 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
		response, err := client.Do(req)
		require.True(t, err == nil, "fetch signed export; URL redacted")
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		_, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition"))
		require.True(t, err == nil, "parse export download disposition")
		require.Equal(t, "验收-fixture.csv", params["filename"])
		content, err := io.ReadAll(io.LimitReader(response.Body, int64(len(payload))+1))
		if err != nil || !bytes.Equal(content, payload) {
			t.Logf("synthetic export expected_bytes=%d received_bytes=%d advertised_bytes=%d read_error_type=%T deadline=%t", len(payload), len(content), response.ContentLength, err, errors.Is(err, context.DeadlineExceeded))
		}
		require.True(t, err == nil && bytes.Equal(content, payload), "full export bytes match including UTF-8 data")
	})
	t.Run("SMTP", func(t *testing.T) {
		server, from, account, token, port := common.SMTPServer, common.SMTPFrom, common.SMTPAccount, common.SMTPToken, common.SMTPPort
		ssl, startTLS, insecure, login := common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPInsecureSkipVerify, common.SMTPForceAuthLogin
		t.Cleanup(func() {
			common.SMTPServer, common.SMTPFrom, common.SMTPAccount, common.SMTPToken, common.SMTPPort = server, from, account, token, port
			common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPInsecureSkipVerify, common.SMTPForceAuthLogin = ssl, startTLS, insecure, login
		})
		common.SMTPServer, common.SMTPFrom = values["SMTPServer"], values["SMTPFrom"]
		common.SMTPAccount, common.SMTPToken = values["SMTPAccount"], values["SMTPToken"]
		common.SMTPPort, err = strconv.Atoi(values["SMTPPort"])
		require.True(t, err == nil, "valid SMTP port")
		common.SMTPSSLEnabled = values["SMTPSSLEnabled"] == "true"
		common.SMTPStartTLSEnabled = values["SMTPStartTLSEnabled"] == "true"
		common.SMTPInsecureSkipVerify = values["SMTPInsecureSkipVerify"] == "true"
		common.SMTPForceAuthLogin = values["SMTPForceAuthLogin"] == "true"
		receiver := common.SMTPFrom
		if receiver == "" {
			receiver = common.SMTPAccount
		}
		address, err := mail.ParseAddress(receiver)
		require.True(t, err == nil, "configured sender must be a valid self-test recipient")
		err = common.SendEmailContext(context.Background(), "[本地验收 / Local acceptance] 系统报告邮件测试", address.Address, "<h2>本地验收 / Local acceptance</h2><p>这是一封经授权的合成测试邮件，不包含客户数据。无需回复。</p><p>Authorized synthetic test. No customer data is included. No reply is needed.</p>")
		if err != nil {
			var protocol *textproto.Error
			var certificate *tls.CertificateVerificationError
			var unknown x509.UnknownAuthorityError
			var network *net.OpError
			switch {
			case errors.As(err, &protocol):
				t.Logf("SMTP protocol rejection code=%d", protocol.Code)
			case errors.As(err, &certificate), errors.As(err, &unknown):
				t.Log("SMTP TLS certificate verification failed")
			case errors.As(err, &network):
				t.Logf("SMTP network failure operation=%s timeout=%t", network.Op, network.Timeout())
			default:
				t.Logf("SMTP error type=%T", err)
			}
		}
		require.True(t, err == nil, "SMTP accepted synthetic self-test message; inbox receipt requires recipient observation")
	})
}
