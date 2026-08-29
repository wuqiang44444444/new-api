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

type assetPolicyRoundTripFunc func(*http.Request) (*http.Response, error)

func (f assetPolicyRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withAssetPolicyHTTPClient(t *testing.T, roundTrip assetPolicyRoundTripFunc) {
	t.Helper()
	previousClient := httpClient
	httpClient = &http.Client{Transport: roundTrip}
	t.Cleanup(func() { httpClient = previousClient })
}

func assetPolicyHTTPResponse(assetID string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"Result":{"Id":%q,"Status":"Active"}}`, assetID))),
	}
}

func withAssetGroupPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelDefaultAssetGroup{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func createMoxingAssetPolicyChannel(t *testing.T, db *gorm.DB, baseURL string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:    constant.ChannelTypeSeedanceLink,
		Status:  common.ChannelStatusEnabled,
		Name:    "Moxing asset policy test",
		Models:  "customer-model",
		Group:   "default",
		Key:     "test-key",
		BaseURL: common.GetPointer(baseURL),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingModelArkV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetMinURLTTLSeconds: 1,
	})
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func generalAssetPolicyRequest(groupID string) dto.CreateAssetRequest {
	return dto.CreateAssetRequest{
		Name: "test", AssetKind: model.AssetKindGeneral, MediaType: "image", Model: "customer-model",
		AssetGroupID: groupID,
		Source:       dto.AssetSource{Type: "url", URL: "https://1.1.1.1/source.png"},
	}
}

func TestCreateRemoteAssetUsesConfiguredDefaultGroupForMissingOrBlankID(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	requests := make(chan map[string]any, 2)
	withAssetPolicyHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, common.DecodeJson(req.Body, &body))
		requests <- body
		return assetPolicyHTTPResponse("asset-1"), nil
	})
	channel := createMoxingAssetPolicyChannel(t, db, "https://upstream.example")
	require.NoError(t, model.SaveChannelDefaultAssetGroup(channel.Id, "system-default-group"))

	for _, groupID := range []string{"", " \t\n "} {
		response, err := CreateRemoteAsset(context.Background(), "default", generalAssetPolicyRequest(groupID))
		require.NoError(t, err)
		assert.Equal(t, "asset-1", response.ID)
		assert.Equal(t, "system-default-group", (<-requests)["GroupId"])
	}
}

func TestCreateRemoteAssetPreservesExplicitGroupWithoutDefault(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	requests := make(chan map[string]any, 1)
	withAssetPolicyHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, common.DecodeJson(req.Body, &body))
		requests <- body
		return assetPolicyHTTPResponse("asset-2"), nil
	})
	createMoxingAssetPolicyChannel(t, db, "https://upstream.example")

	explicitID := " provider-opaque-group "
	response, err := CreateRemoteAsset(context.Background(), "default", generalAssetPolicyRequest(explicitID))

	require.NoError(t, err)
	assert.Equal(t, "asset-2", response.ID)
	assert.Equal(t, explicitID, (<-requests)["GroupId"])
}

func TestCreateRemoteAssetFailsClosedWhenDefaultGroupIsMissing(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	requestCount := 0
	withAssetPolicyHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		requestCount++
		return assetPolicyHTTPResponse("unexpected"), nil
	})
	createMoxingAssetPolicyChannel(t, db, "https://upstream.example")

	_, err := CreateRemoteAsset(context.Background(), "default", generalAssetPolicyRequest(""))

	require.ErrorIs(t, err, ErrDefaultAssetGroupNotConfigured)
	assert.Zero(t, requestCount)
}

func TestResolveAssetGroupIDKeepsRealPersonSeparateAndNoneIgnoresField(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})

	groupID, err := resolveAssetGroupID(channel, model.AssetKindGeneral, "caller-group")
	require.NoError(t, err)
	assert.Empty(t, groupID)

	_, err = resolveAssetGroupID(channel, model.AssetKindRealPerson, "  ")
	require.ErrorIs(t, err, ErrInvalidAssetRequest)

	groupID, err = resolveAssetGroupID(channel, model.AssetKindRealPerson, " real-person-group ")
	require.NoError(t, err)
	assert.Equal(t, " real-person-group ", groupID)
}

func TestCreateAssetGroupRejectsReservedGeneralNameBeforeRouting(t *testing.T) {
	_, err := CreateAssetGroup(context.Background(), "default", dto.CreateAssetGroupRequest{
		Name: DefaultAssetGroupName, GroupKind: model.AssetKindGeneral, Model: "customer-model",
	})

	require.ErrorIs(t, err, ErrReservedAssetGroupName)
}
