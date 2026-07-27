package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowedH5URLUsesExplicitHTTPSHostAllowlist(t *testing.T) {
	assert.True(t, allowedH5URL("https://moxing.example/h5/session", "https://moxing.example", nil))
	assert.True(t, allowedH5URL("https://verify.example/h5/session", "https://moxing.example", []string{"verify.example"}))
	assert.False(t, allowedH5URL("http://moxing.example/h5/session", "https://moxing.example", nil))
	assert.False(t, allowedH5URL("https://evil.example/h5/session", "https://moxing.example", nil))
	assert.False(t, allowedH5URL("https://user:pass@moxing.example/h5/session", "https://moxing.example", nil))
}

func TestVerificationPollFinishesWhenAuthorizationIsTerminal(t *testing.T) {
	truncate(t)
	authorization := model.RealPersonAuthorization{UserID: 101, Status: model.RealPersonAuthorizationAuthorized}
	require.NoError(t, model.DB.Create(&authorization).Error)
	job := model.AssetOperationJob{IdempotencyKey: "poll-terminal", Kind: "poll_verification", AuthorizationID: &authorization.ID, Status: model.AssetJobRunning}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, processRealPersonVerificationPoll(context.Background(), &job))
	require.NoError(t, model.DB.First(&job, job.ID).Error)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestConsentExpiryTransitionUsesCAS(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	expiring := model.RealPersonAuthorization{
		UserID: 100, Status: model.RealPersonAuthorizationAwaitingConsent,
		ConsentTokenHash: "consent-expiring", ConsentTokenExpiresAt: now - 1,
	}
	rejected := model.RealPersonAuthorization{
		UserID: 100, Status: model.RealPersonAuthorizationConsentRejected,
		ConsentTokenHash: "consent-already-rejected", ConsentTokenExpiresAt: now - 1,
	}
	require.NoError(t, model.DB.Create(&expiring).Error)
	require.NoError(t, model.DB.Create(&rejected).Error)

	changed, err := expireAwaitingRealPersonConsent(context.Background(), &expiring, now)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, model.RealPersonAuthorizationExpired, expiring.Status)

	staleRejected := rejected
	staleRejected.Status = model.RealPersonAuthorizationAwaitingConsent
	changed, err = expireAwaitingRealPersonConsent(context.Background(), &staleRejected, now)
	require.NoError(t, err)
	assert.False(t, changed)
	require.NoError(t, model.DB.First(&rejected, "id = ?", rejected.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationConsentRejected, rejected.Status)
}

func TestConsentEvidenceHMACBindsPolicyAndAuthorization(t *testing.T) {
	first := consentEvidenceHMAC("secret", "rpa_1", "policy-a", "ua", "192.0.2.1", 100)
	assert.Equal(t, first, consentEvidenceHMAC("secret", "rpa_1", "policy-a", "ua", "192.0.2.1", 100))
	assert.NotEqual(t, first, consentEvidenceHMAC("secret", "rpa_2", "policy-a", "ua", "192.0.2.1", 100))
	assert.NotEqual(t, first, consentEvidenceHMAC("secret", "rpa_1", "policy-b", "ua", "192.0.2.1", 100))
	assert.NotEqual(t, first, consentEvidenceHMAC("secret", "rpa_1", "policy-a", "ua", "192.0.2.2", 100))
}

func TestVerificationRefreshDoesNotReviveRevokedAuthorization(t *testing.T) {
	authorization, session := seedArkVerificationState(t, model.RealPersonAuthorizationVerifying)
	staleAuthorization := *authorization
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&model.RealPersonAuthorization{}).Where("id = ?", authorization.ID).Updates(map[string]any{
		"status": model.RealPersonAuthorizationRevoked, "revoked_at": now, "updated_at": now,
	}).Error)

	require.NoError(t, RefreshRealPersonVerification(context.Background(), &staleAuthorization))
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(session, "id = ?", session.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, authorization.Status)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, staleAuthorization.Status)
	assert.Equal(t, "verifying", session.Status)
	assert.Empty(t, session.UpstreamGroupID)
	var groupCount int64
	require.NoError(t, model.DB.Model(&model.AssetGroupBinding{}).Where("authorization_id = ?", authorization.ID).Count(&groupCount).Error)
	assert.Zero(t, groupCount)
}

func TestVerificationRefreshCommitsAuthorizationSessionAndGroup(t *testing.T) {
	authorization, session := seedArkVerificationState(t, model.RealPersonAuthorizationVerifying)

	require.NoError(t, RefreshRealPersonVerification(context.Background(), authorization))
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(session, "id = ?", session.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationAuthorized, authorization.Status)
	assert.Equal(t, "active", session.Status)
	assert.Equal(t, "group-1", session.UpstreamGroupID)
	var group model.AssetGroupBinding
	require.NoError(t, model.DB.First(&group, "authorization_id = ?", authorization.ID).Error)
	assert.Equal(t, model.AssetBindingStatusActive, group.Status)
	assert.Equal(t, "group-1", group.UpstreamResourceID)
}

func TestVerificationRefreshExposesStableRejectedErrorCode(t *testing.T) {
	authorization, session := seedArkVerificationState(t, model.RealPersonAuthorizationVerifying)
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"session_id":"session-1","status":"rejected"}`)),
		}, nil
	})}

	require.NoError(t, RefreshRealPersonVerification(context.Background(), authorization))
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(session, "id = ?", session.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationFailed, authorization.Status)
	assert.Equal(t, "real_person_verification_rejected", authorization.ErrorCode)
	assert.Equal(t, "rejected", session.Status)
	assert.Equal(t, "real_person_verification_rejected", session.ErrorCode)
}

func TestVerificationPollFinishesAfterProviderExpiresAuthorization(t *testing.T) {
	authorization, session := seedArkVerificationState(t, model.RealPersonAuthorizationVerifying)
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"session_id":"session-1","status":"expired"}`)),
		}, nil
	})}
	job := model.AssetOperationJob{
		IdempotencyKey: "poll-provider-expired", Kind: "poll_verification", AuthorizationID: &authorization.ID,
		Status: model.AssetJobRunning, LockedBy: "provider-expired-runner", MaxAttempts: 10,
	}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, processRealPersonVerificationPoll(context.Background(), &job))
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(session, "id = ?", session.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationExpired, authorization.Status)
	assert.Equal(t, "real_person_verification_expired", authorization.ErrorCode)
	assert.Equal(t, "expired", session.Status)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestVerificationPollExpiryCASDoesNotOverwriteRevocation(t *testing.T) {
	truncate(t)
	authorization := model.RealPersonAuthorization{
		UserID: 102, Status: model.RealPersonAuthorizationRevoked, RevokedAt: common.GetTimestamp(),
		ConsentTokenHash: "poll-expiry-revoked",
	}
	require.NoError(t, model.DB.Create(&authorization).Error)
	tokenHash := "poll-expiry-secret"
	session := model.RealPersonVerificationSession{
		AuthorizationID: authorization.ID, Status: "verifying",
		VerificationHandleCiphertext: "encrypted-handle",
		H5URLCiphertext:              "encrypted-url",
		VerificationTokenHash:        &tokenHash,
	}
	require.NoError(t, model.DB.Create(&session).Error)
	job := model.AssetOperationJob{
		IdempotencyKey: "poll-expiry-cas", Kind: "poll_verification", AuthorizationID: &authorization.ID,
		Status: model.AssetJobRunning, LockedBy: "expiry-cas-runner",
	}
	require.NoError(t, model.DB.Create(&job).Error)
	staleAuthorization := authorization
	staleAuthorization.Status = model.RealPersonAuthorizationVerifying
	staleAuthorization.RevokedAt = 0

	require.NoError(t, expireRealPersonVerificationPoll(&job, &staleAuthorization))
	require.NoError(t, model.DB.First(&authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	require.NoError(t, model.DB.First(&session, "id = ?", session.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, authorization.Status)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, staleAuthorization.Status)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
	assert.Empty(t, session.VerificationHandleCiphertext)
	assert.Empty(t, session.H5URLCiphertext)
	assert.Nil(t, session.VerificationTokenHash)
}

func TestVerificationSessionCreationDoesNotReviveRevokedAuthorization(t *testing.T) {
	truncate(t)
	originalHTTPClient := httpClient
	var createCalls atomic.Int32
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		createCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"session_id":"session-new","h5_link":"https://ark.example/h5","status":"verifying"}`)),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })
	baseURL := "https://ark.example"
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "ark-verification", BaseURL: &baseURL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"ark_assets","video_upstream_profile":"third_party_reverse_proxy"}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	authorization := model.RealPersonAuthorization{
		UserID: 204, RequestedModel: "video-model", ChannelID: channel.Id,
		CredentialFingerprint: model.AssetCredentialFingerprint(baseURL, "single-key", string(dto.AssetUpstreamProfileArk)),
		UpstreamProfile:       string(dto.AssetUpstreamProfileArk), Status: model.RealPersonAuthorizationRevoked,
		ConsentTokenHash: "verification-session-revoked", RevokedAt: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&authorization).Error)
	staleAuthorization := authorization
	staleAuthorization.Status = model.RealPersonAuthorizationFailed
	staleAuthorization.RevokedAt = 0

	_, err := createRealPersonVerificationSession(context.Background(), &staleAuthorization)
	require.ErrorContains(t, err, "authorization state changed")
	require.NoError(t, model.DB.First(&authorization, "id = ?", authorization.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, authorization.Status)
	var sessions, jobs int64
	require.NoError(t, model.DB.Model(&model.RealPersonVerificationSession{}).Where("authorization_id = ?", authorization.ID).Count(&sessions).Error)
	require.NoError(t, model.DB.Model(&model.AssetOperationJob{}).Where("authorization_id = ?", authorization.ID).Count(&jobs).Error)
	assert.Zero(t, sessions)
	assert.Zero(t, jobs)
	assert.Zero(t, createCalls.Load())
}

func TestVerificationSessionUnknownCreateWaitsForTTLBeforeRetry(t *testing.T) {
	authorization := seedArkRetryAuthorization(t, 205, "verification-create-unknown")
	originalHTTPClient := httpClient
	var createCalls atomic.Int32
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		createCalls.Add(1)
		return nil, errors.New("verification create timeout")
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })

	_, err := RetryRealPersonVerification(context.Background(), authorization)
	require.Error(t, err)
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationVerifying, authorization.Status)

	var session model.RealPersonVerificationSession
	require.NoError(t, model.DB.Where("authorization_id = ?", authorization.ID).Order("id desc").First(&session).Error)
	assert.Equal(t, realPersonVerificationSessionCreateUnknown, session.Status)
	assert.Equal(t, realPersonVerificationCreateUnknownCode, session.ErrorCode)
	assert.Greater(t, session.ExpiresAt, common.GetTimestamp())
	var job model.AssetOperationJob
	require.NoError(t, model.DB.First(&job, "authorization_id = ? AND kind = ?", authorization.ID, "poll_verification").Error)
	assert.Equal(t, model.AssetJobPending, job.Status)
	assert.Equal(t, int32(1), createCalls.Load())

	_, err = RetryRealPersonVerification(context.Background(), authorization)
	require.ErrorContains(t, err, "not retryable")
	assert.Equal(t, int32(1), createCalls.Load())

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Model(&session).Updates(map[string]any{"expires_at": now - 1, "updated_at": now}).Error)
	require.NoError(t, model.DB.Model(&job).Updates(map[string]any{"status": model.AssetJobRunning, "locked_by": "unknown-create-runner", "locked_until": now + 60, "updated_at": now}).Error)
	job.Status = model.AssetJobRunning
	job.LockedBy = "unknown-create-runner"
	require.NoError(t, processRealPersonVerificationPoll(context.Background(), &job))
	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationFailed, authorization.Status)
	assert.Equal(t, realPersonVerificationCreateUnknownCode, authorization.ErrorCode)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestVerificationSessionDefinitiveCreateFailuresCloseClaim(t *testing.T) {
	tests := []struct {
		name       string
		tokenHash  string
		statusCode int
		body       string
	}{
		{name: "upstream rejection", tokenHash: "verification-rejected-create", statusCode: http.StatusBadRequest, body: `{}`},
		{name: "successful response without session id", tokenHash: "verification-missing-session-id", statusCode: http.StatusOK, body: `{}`},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorization := seedArkRetryAuthorization(t, 210+index, test.tokenHash)
			originalHTTPClient := httpClient
			httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.statusCode,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(test.body)),
				}, nil
			})}
			t.Cleanup(func() { httpClient = originalHTTPClient })

			_, err := RetryRealPersonVerification(context.Background(), authorization)
			require.Error(t, err)
			require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
			assert.Equal(t, model.RealPersonAuthorizationFailed, authorization.Status)
			assert.Equal(t, realPersonVerificationSessionFailedCode, authorization.ErrorCode)
			var session model.RealPersonVerificationSession
			require.NoError(t, model.DB.First(&session, "authorization_id = ?", authorization.ID).Error)
			assert.Equal(t, "failed", session.Status)
			assert.Equal(t, realPersonVerificationSessionFailedCode, session.ErrorCode)
			var job model.AssetOperationJob
			require.NoError(t, model.DB.First(&job, "authorization_id = ? AND kind = ?", authorization.ID, "poll_verification").Error)
			assert.Equal(t, model.AssetJobSucceeded, job.Status)
		})
	}
}

func TestConcurrentVerificationRetryCreatesOnlyOneUpstreamSession(t *testing.T) {
	authorization := seedArkRetryAuthorization(t, 206, "verification-concurrent-retry")
	entered, release, createCalls := useBlockingVerificationCreate(t)
	firstAuthorization := *authorization
	firstResult := make(chan error, 1)
	go func() {
		_, err := RetryRealPersonVerification(context.Background(), &firstAuthorization)
		firstResult <- err
	}()
	<-entered

	secondAuthorization := *authorization
	_, err := RetryRealPersonVerification(context.Background(), &secondAuthorization)
	require.ErrorContains(t, err, "authorization state changed")
	assert.Equal(t, int32(1), createCalls.Load())
	release()
	require.NoError(t, <-firstResult)
	assert.Equal(t, int32(1), createCalls.Load())

	var sessions int64
	require.NoError(t, model.DB.Model(&model.RealPersonVerificationSession{}).Where("authorization_id = ?", authorization.ID).Count(&sessions).Error)
	assert.Equal(t, int64(1), sessions)
}

func TestVerificationCreateResultAfterRevokeIsRecordedAsOrphaned(t *testing.T) {
	authorization := seedArkRetryAuthorization(t, 207, "verification-concurrent-revoke")
	entered, release, createCalls := useBlockingVerificationCreate(t)
	createAuthorization := *authorization
	createResult := make(chan error, 1)
	go func() {
		_, err := RetryRealPersonVerification(context.Background(), &createAuthorization)
		createResult <- err
	}()
	<-entered

	var current model.RealPersonAuthorization
	require.NoError(t, model.DB.First(&current, "id = ?", authorization.ID).Error)
	require.NoError(t, RevokeRealPersonAuthorization(context.Background(), &current))
	release()
	require.ErrorContains(t, <-createResult, "authorization state changed")
	assert.Equal(t, int32(1), createCalls.Load())

	require.NoError(t, model.DB.First(authorization, "id = ?", authorization.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationDeleted, authorization.Status)
	var session model.RealPersonVerificationSession
	require.NoError(t, model.DB.Where("authorization_id = ?", authorization.ID).Order("id desc").First(&session).Error)
	assert.Equal(t, "session-new", session.UpstreamSessionID)
	assert.Equal(t, realPersonVerificationSessionOrphaned, session.Status)
	assert.Equal(t, realPersonVerificationSessionOrphanedErrorCode, session.ErrorCode)
	assert.Empty(t, session.VerificationHandleCiphertext)
	assert.Empty(t, session.H5URLCiphertext)
	assert.Nil(t, session.VerificationTokenHash)
	var job model.AssetOperationJob
	require.NoError(t, model.DB.First(&job, "authorization_id = ? AND kind = ?", authorization.ID, "poll_verification").Error)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestRevokeRealPersonAuthorizationFinalizesWithoutResources(t *testing.T) {
	truncate(t)
	authorization := model.RealPersonAuthorization{
		UserID: 201, Status: model.RealPersonAuthorizationAuthorized,
		ConsentTokenHash: "revoke-empty-authorization",
	}
	require.NoError(t, model.DB.Create(&authorization).Error)
	tokenHash := "revoke-empty-secret"
	session := model.RealPersonVerificationSession{
		AuthorizationID: authorization.ID, Status: "verifying",
		VerificationHandleCiphertext: "encrypted-handle",
		H5URLCiphertext:              "encrypted-url",
		VerificationTokenHash:        &tokenHash,
	}
	require.NoError(t, model.DB.Create(&session).Error)

	require.NoError(t, RevokeRealPersonAuthorization(context.Background(), &authorization))
	require.NoError(t, model.DB.First(&authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(&session, "id = ?", session.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationDeleted, authorization.Status)
	assert.NotZero(t, authorization.RevokedAt)
	assert.Empty(t, session.VerificationHandleCiphertext)
	assert.Empty(t, session.H5URLCiphertext)
	assert.Nil(t, session.VerificationTokenHash)
}

func TestRevokeKeepsDeletingBindingPending(t *testing.T) {
	truncate(t)
	authorization := model.RealPersonAuthorization{
		UserID: 202, Status: model.RealPersonAuthorizationAuthorized,
		ConsentTokenHash: "revoke-deleting-binding",
	}
	require.NoError(t, model.DB.Create(&authorization).Error)
	asset := model.Asset{UserID: 202, Name: "real person", AssetKind: model.AssetKindRealPerson, MediaType: "image", Status: model.AssetStatusDeleting, AuthorizationID: &authorization.ID}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: 202, ChannelID: 3, CredentialFingerprint: "credential", UpstreamProfile: string(dto.AssetUpstreamProfileArk), Status: model.AssetBindingStatusDeleting}
	require.NoError(t, model.DB.Create(&binding).Error)

	require.NoError(t, RevokeRealPersonAuthorization(context.Background(), &authorization))
	require.NoError(t, model.DB.First(&authorization, "id = ?", authorization.ID).Error)
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	assert.Equal(t, model.RealPersonAuthorizationRevoked, authorization.Status)
	assert.Equal(t, model.AssetStatusDeleting, asset.Status)
	assert.Zero(t, asset.DeletedAt)
	var job model.AssetOperationJob
	require.NoError(t, model.DB.First(&job, "binding_id = ? AND kind = ?", binding.ID, "delete_binding").Error)
	assert.Equal(t, model.AssetJobPending, job.Status)
}

func seedArkVerificationState(t *testing.T, status string) (*model.RealPersonAuthorization, *model.RealPersonVerificationSession) {
	t.Helper()
	truncate(t)
	originalHTTPClient := httpClient
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"session_id":"session-1","group_id":"group-1","status":"group_ready"}`)),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })
	baseURL := "https://ark.example"
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "ark-verification", BaseURL: &baseURL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"ark_assets","video_upstream_profile":"third_party_reverse_proxy"}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	authorization := &model.RealPersonAuthorization{
		UserID: 203, RequestedModel: "video-model", ChannelID: channel.Id,
		CredentialFingerprint: model.AssetCredentialFingerprint(baseURL, "single-key", string(dto.AssetUpstreamProfileArk)),
		UpstreamProfile:       string(dto.AssetUpstreamProfileArk), Status: status,
		ConsentTokenHash: "verification-" + t.Name(),
	}
	require.NoError(t, model.DB.Create(authorization).Error)
	session := &model.RealPersonVerificationSession{AuthorizationID: authorization.ID, UpstreamSessionID: "session-1", Status: "verifying"}
	require.NoError(t, model.DB.Create(session).Error)
	return authorization, session
}

func seedArkRetryAuthorization(t *testing.T, userID int, consentTokenHash string) *model.RealPersonAuthorization {
	t.Helper()
	truncate(t)
	baseURL := "https://ark.example"
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "ark-verification", BaseURL: &baseURL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"ark_assets","video_upstream_profile":"third_party_reverse_proxy"}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	authorization := &model.RealPersonAuthorization{
		UserID: userID, RequestedModel: "video-model", ChannelID: channel.Id,
		CredentialFingerprint: model.AssetCredentialFingerprint(baseURL, "single-key", string(dto.AssetUpstreamProfileArk)),
		UpstreamProfile:       string(dto.AssetUpstreamProfileArk), Status: model.RealPersonAuthorizationFailed,
		ConsentTokenHash: consentTokenHash, ConsentedAt: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(authorization).Error)
	return authorization
}

func useBlockingVerificationCreate(t *testing.T) (<-chan struct{}, func(), *atomic.Int32) {
	t.Helper()
	originalHTTPClient := httpClient
	entered := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseUpstream) }) }
	createCalls := &atomic.Int32{}
	httpClient = &http.Client{Transport: assetVerificationRoundTripper(func(*http.Request) (*http.Response, error) {
		if createCalls.Add(1) == 1 {
			close(entered)
		}
		<-releaseUpstream
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"session_id":"session-new","h5_link":"https://ark.example/h5","status":"verifying"}`)),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })
	t.Cleanup(release)
	return entered, release, createCalls
}

type assetVerificationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip assetVerificationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
