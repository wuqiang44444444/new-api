package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type assetServiceRoundTripFunc func(*http.Request) (*http.Response, error)

func (function assetServiceRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func withAssetServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Asset{}, &model.AssetGroup{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})
	return db
}

func createRelayAssetChannel(t *testing.T, db *gorm.DB, baseURL string) (*model.Channel, string) {
	t.Helper()
	channel := &model.Channel{
		Type:    constant.ChannelTypeSeedanceLink,
		Status:  common.ChannelStatusEnabled,
		Name:    "asset relay",
		Models:  "seedance-assets",
		Group:   "default",
		Key:     "old-key",
		BaseURL: common.GetPointer(baseURL),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolRelayAssetsV1,
		AssetMinURLTTLSeconds: 3600,
	})
	require.NoError(t, db.Create(channel).Error)
	_, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	require.NoError(t, err)
	return channel, fingerprint
}

func TestCreateRemoteAssetDistinguishesChannelAndScopeConflicts(t *testing.T) {
	channel := &model.Channel{Id: 91}
	fingerprint := "scope-a"
	request := dto.CreateAssetRequest{
		Name:         "source",
		AssetKind:    model.AssetKindGeneral,
		MediaType:    "image",
		Model:        "seedance-assets",
		AssetGroupID: "astgrp_contract",
	}
	group := &model.AssetGroup{
		PublicID:              request.AssetGroupID,
		UserID:                1,
		CreatedByTokenID:      2,
		AppID:                 2,
		Name:                  "group",
		GroupKind:             model.AssetKindGeneral,
		RequestedModel:        "another-model",
		ChannelID:             channel.Id,
		CredentialFingerprint: fingerprint,
		UpstreamProtocol:      string(dto.AssetUpstreamProtocolRelayAssetsV1),
		Status:                model.AssetStatusReady,
		UpstreamResourceID:    "group-upstream",
	}
	settings := dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocolRelayAssetsV1}
	err := validateAssetGroupBinding(group, request, channel, fingerprint, settings)
	assert.ErrorIs(t, err, ErrAssetChannelMismatch)

	group.RequestedModel = request.Model
	group.CredentialFingerprint = "different-scope"
	err = validateAssetGroupBinding(group, request, channel, fingerprint, settings)
	assert.ErrorIs(t, err, ErrAssetScopeConflict)
}

func TestRefreshAssetTreatsUpstream404AsDeleted(t *testing.T) {
	db := withAssetServiceDB(t)
	previousHTTPClient := httpClient
	httpClient = &http.Client{Transport: assetServiceRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"detail":"missing"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { httpClient = previousHTTPClient })
	channel, fingerprint := createRelayAssetChannel(t, db, "https://relay.example.com")
	asset := &model.Asset{
		PublicID:              "ast_missing",
		UserID:                1,
		CreatedByTokenID:      2,
		AppID:                 2,
		Name:                  "missing",
		AssetKind:             model.AssetKindGeneral,
		MediaType:             "image",
		RequestedModel:        "seedance-assets",
		ChannelID:             channel.Id,
		CredentialFingerprint: fingerprint,
		UpstreamProtocol:      string(dto.AssetUpstreamProtocolRelayAssetsV1),
		UpstreamResourceID:    "provider-missing",
		Status:                model.AssetStatusReady,
	}
	require.NoError(t, db.Create(asset).Error)

	err := RefreshAsset(context.Background(), asset)
	assert.ErrorIs(t, err, ErrAssetNotFound)
	require.NoError(t, db.First(asset, asset.ID).Error)
	assert.Equal(t, model.AssetStatusDeleted, asset.Status)
	assert.NotZero(t, asset.DeletedAt)
}
