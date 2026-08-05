package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteAssetURLValidationDoesNotFetchAndRejectsNonPublicTargets(t *testing.T) {
	url, err := validateRemoteAssetURL("https://8.8.8.8/reference.png?signature=secret", 8192)
	require.NoError(t, err)
	assert.Contains(t, url, "signature=secret")

	_, err = validateRemoteAssetURL("https://169.254.169.254/latest/meta-data", 8192)
	assert.ErrorIs(t, err, ErrUnsafeAssetURL)
	_, err = validateRemoteAssetURL("https://user:password@8.8.8.8/source", 8192)
	assert.ErrorIs(t, err, ErrUnsafeAssetURL)
}

func TestRemoteAssetURLTTLReportsProviderMinimum(t *testing.T) {
	now := time.Unix(1_000, 0)
	err := validateRemoteAssetTTL(1_100, 300, now)
	assert.ErrorIs(t, err, ErrAssetURLTTLInsufficient)
	required, ok := RequiredAssetURLTTL(err)
	assert.True(t, ok)
	assert.Equal(t, int64(300), required)
	assert.NoError(t, validateRemoteAssetTTL(1_300, 300, now))
	assert.NoError(t, validateRemoteAssetTTL(0, 300, now))
}

func TestIdempotentRemoteCreateValidatesSourceTypeBeforeURL(t *testing.T) {
	_, err := CreateRemoteAsset(context.Background(), 1, 1, "default", "default", "source-type", dto.CreateAssetRequest{
		Name: "invalid", AssetKind: model.AssetKindGeneral, MediaType: "image",
		Source: dto.AssetSource{Type: "direct_upload"},
	})
	assert.ErrorIs(t, err, ErrAssetURLRequired)
}

func TestCreateRemoteAssetImmediatelyBindsWithoutManagedStorage(t *testing.T) {
	truncate(t)
	assetSettings := asset_setting.GetBusinessSetting()
	originalAssetEnabled := assetSettings.Enabled
	assetSettings.Enabled = true
	t.Cleanup(func() { assetSettings.Enabled = originalAssetEnabled })
	seedUser(t, 905, 0)
	InitHttpClient()

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/joycreator/openApi/v1/asset/group/create":
			_, _ = w.Write([]byte(`{"requestId":"group-request","result":{"group":{"id":"group-resource","groupId":"group-business","status":1}}}`))
		case "/joycreator/openApi/v1/asset/create":
			_, _ = w.Write([]byte(`{"requestId":"asset-request","result":{"asset":{"id":"asset-resource","assetId":"asset-business","vendorUrl":"https://provider.example/signed-result","vendorStatus":"Active","status":1}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "remote-assets", BaseURL: &server.URL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"joycreator_assets","asset_min_url_ttl_seconds":3600}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	req := dto.CreateAssetRequest{
		Name: "remote image", AssetKind: model.AssetKindGeneral, MediaType: "image", Target: assetBindingTargetJoyCreator,
		Source: dto.AssetSource{Type: "url", URL: "https://8.8.8.8/reference.png?signature=secret", ExpiresAt: time.Now().Add(2 * time.Hour).Unix()},
	}
	asset, err := CreateRemoteAsset(context.Background(), 905, 12, "default", "default", "remote-create-1", req)
	require.NoError(t, err)
	assert.Equal(t, model.AssetStatusReady, asset.Status)

	var binding model.AssetBinding
	require.NoError(t, model.DB.First(&binding, "asset_id = ?", asset.ID).Error)
	assert.Equal(t, model.AssetBindingStatusActive, binding.Status)
	assert.Equal(t, "asset-resource", binding.UpstreamResourceID)
	var watchdog model.AssetOperationJob
	require.NoError(t, model.DB.First(&watchdog, "idempotency_key = ?", remoteCreateWatchdogKey(binding.ID)).Error)
	assert.Equal(t, model.AssetJobSucceeded, watchdog.Status)

	assetSettings.Enabled = false
	replayed, err := CreateRemoteAsset(context.Background(), 905, 12, "default", "default", "remote-create-1", req)
	require.NoError(t, err)
	assert.Equal(t, asset.PublicID, replayed.PublicID)
	assert.Equal(t, int32(2), upstreamCalls.Load())
}

func TestCreateRemoteAssetReturnsUpstreamErrorForExplicitFailedResult(t *testing.T) {
	truncate(t)
	seedUser(t, 907, 0)
	InitHttpClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/joycreator/openApi/v1/asset/group/create":
			_, _ = w.Write([]byte(`{"requestId":"group-request","result":{"group":{"id":"group-resource","groupId":"group-business","status":1}}}`))
		case "/joycreator/openApi/v1/asset/create":
			_, _ = w.Write([]byte(`{"requestId":"asset-request","result":{"asset":{"id":"asset-resource","assetId":"asset-business","status":2,"errorMsg":"provider rejected content"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "remote-assets-failed", BaseURL: &server.URL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"joycreator_assets","asset_min_url_ttl_seconds":3600}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	req := dto.CreateAssetRequest{
		Name: "remote image", AssetKind: model.AssetKindGeneral, MediaType: "image", Target: assetBindingTargetJoyCreator,
		Source: dto.AssetSource{Type: "url", URL: "https://8.8.8.8/reference.png", ExpiresAt: time.Now().Add(2 * time.Hour).Unix()},
	}
	asset, err := CreateRemoteAsset(context.Background(), 907, 12, "default", "default", "remote-create-failed", req)
	assert.ErrorIs(t, err, ErrAssetUpstreamError)
	assert.Nil(t, asset)

	var persisted model.Asset
	require.NoError(t, model.DB.First(&persisted, "user_id = ?", 907).Error)
	assert.Equal(t, model.AssetStatusFailed, persisted.Status)
}

func TestRemoteCreateResultStoresOnlyControlPlaneMapping(t *testing.T) {
	truncate(t)
	seedUser(t, 901, 0)
	asset := model.Asset{UserID: 901, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusCreating}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: 901, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", RequestedModel: "video-model", Status: model.AssetBindingStatusCreating}
	require.NoError(t, model.DB.Create(&binding).Error)

	err := saveRemoteCreateResult(&asset, &binding, assetadapter.AssetResult{ResourceID: "resource-1", BusinessID: "asset-1", ReferenceType: "asset_uri_id", ReferenceValue: "asset-1", Status: "active"})
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	assert.Equal(t, model.AssetStatusReady, asset.Status)
	assert.Equal(t, model.AssetBindingStatusActive, binding.Status)
	assert.Equal(t, "asset-1", binding.UpstreamReferenceValue)
}

func TestRemoteCreateUnknownGroupOutcomeReturnsReplayableAsset(t *testing.T) {
	truncate(t)
	seedUser(t, 906, 0)
	InitHttpClient()

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Key: "single-key", Status: common.ChannelStatusEnabled,
		Name: "unknown-group", BaseURL: &server.URL, Group: "default",
		OtherSettings: `{"asset_upstream_profile":"joycreator_assets","asset_min_url_ttl_seconds":3600}`,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	req := dto.CreateAssetRequest{
		Name: "remote image", AssetKind: model.AssetKindGeneral, MediaType: "image", Target: assetBindingTargetJoyCreator,
		Source: dto.AssetSource{Type: "url", URL: "https://8.8.8.8/reference.png", ExpiresAt: time.Now().Add(2 * time.Hour).Unix()},
	}

	asset, err := CreateRemoteAsset(context.Background(), 906, 12, "default", "default", "unknown-group-create", req)
	require.NoError(t, err)
	assert.Equal(t, model.AssetStatusCreateUnknown, asset.Status)
	replayed, err := CreateRemoteAsset(context.Background(), 906, 12, "default", "default", "unknown-group-create", req)
	require.NoError(t, err)
	assert.Equal(t, asset.ID, replayed.ID)
	assert.Equal(t, int32(1), upstreamCalls.Load())
}

func TestAssetDeletionWithoutBindingsCompletesLocally(t *testing.T) {
	truncate(t)
	seedUser(t, 902, 0)
	asset := model.Asset{UserID: 902, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusReady}
	require.NoError(t, model.DB.Create(&asset).Error)

	require.NoError(t, DeleteAssetForApp(context.Background(), 902, asset.AppID, asset.PublicID))
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	assert.Equal(t, model.AssetStatusDeleted, asset.Status)
	assert.NotZero(t, asset.DeletedAt)
	var jobs int64
	require.NoError(t, model.DB.Model(&model.AssetOperationJob{}).Where("asset_id = ?", asset.ID).Count(&jobs).Error)
	assert.Zero(t, jobs)
}
