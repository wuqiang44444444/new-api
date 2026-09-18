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
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
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
	pinPublishedSeedanceServiceArtifact(t)
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
		response, err := CreateRemoteAsset(context.Background(), "default", 1, generalAssetPolicyRequest(groupID))
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
	response, err := CreateRemoteAsset(context.Background(), "default", 1, generalAssetPolicyRequest(explicitID))

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

	_, err := CreateRemoteAsset(context.Background(), "default", 1, generalAssetPolicyRequest(""))

	require.ErrorIs(t, err, ErrDefaultAssetGroupNotConfigured)
	assert.Zero(t, requestCount)
}

func TestResolveAssetGroupIDKeepsRealPersonSeparateAndNoneIgnoresField(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})

	groupID, err := resolveAssetGroupID(channel, model.AssetKindGeneral, "caller-group", channel.GetOtherSettings().AssetUpstreamProtocol.GeneralAssetGroupPolicy())
	require.NoError(t, err)
	assert.Empty(t, groupID)

	_, err = resolveAssetGroupID(channel, model.AssetKindRealPerson, "  ", channel.GetOtherSettings().AssetUpstreamProtocol.GeneralAssetGroupPolicy())
	require.ErrorIs(t, err, ErrInvalidAssetRequest)

	groupID, err = resolveAssetGroupID(channel, model.AssetKindRealPerson, " real-person-group ", channel.GetOtherSettings().AssetUpstreamProtocol.GeneralAssetGroupPolicy())
	require.NoError(t, err)
	assert.Equal(t, " real-person-group ", groupID)
}

// adapterWithoutDeclaredPolicy models an adapter type that does not carry a
// declared group policy: the legacy Go protocol table must not answer for it.
type adapterWithoutDeclaredPolicy struct {
	assetadapter.Adapter
}

func TestSeedanceAssetGroupPolicyRequiresDeclaredSource(t *testing.T) {
	_, err := seedanceAssetGroupPolicy(adapterWithoutDeclaredPolicy{Adapter: hostedPolicyAdapterStub{}})
	require.ErrorIs(t, err, ErrAssetUpstreamUnavailable)

	policy, err := seedanceAssetGroupPolicy(hostedPolicyAdapterStub{})
	require.NoError(t, err)
	assert.Equal(t, dto.GeneralAssetGroupPolicyHosted, policy)
}

type hostedPolicyAdapterStub struct{}

func (hostedPolicyAdapterStub) Profile() dto.AssetUpstreamProfile {
	return kitdto.AssetUpstreamProfileFunCloudHosted
}

func (hostedPolicyAdapterStub) Supports(kind, mediaType string) bool {
	return kind == "general" && mediaType == "image"
}

func (hostedPolicyAdapterStub) CreateAsset(context.Context, assetadapter.AssetRequest) (assetadapter.AssetResult, error) {
	return assetadapter.AssetResult{}, nil
}

func (hostedPolicyAdapterStub) GetAsset(_ context.Context, _ string) (assetadapter.AssetResult, error) {
	return assetadapter.AssetResult{}, nil
}

func (hostedPolicyAdapterStub) UpdateAsset(_ context.Context, _ string, _ string) (assetadapter.AssetResult, error) {
	return assetadapter.AssetResult{}, nil
}

func (hostedPolicyAdapterStub) DeleteAsset(_ context.Context, _ string) error { return nil }

func (hostedPolicyAdapterStub) GeneralAssetGroupPolicy() dto.GeneralAssetGroupPolicy {
	return dto.GeneralAssetGroupPolicyHosted
}

func TestCreateAssetGroupRejectsReservedGeneralNameBeforeRouting(t *testing.T) {
	_, err := CreateAssetGroup(context.Background(), "default", 1, dto.CreateAssetGroupRequest{
		Name: DefaultAssetGroupName, GroupKind: model.AssetKindGeneral, Model: "customer-model",
	})

	require.ErrorIs(t, err, ErrReservedAssetGroupName)
}

// A cancelled source read is a request failure, never a missing multipart source
// inside the FunCloud adapter. This exercises the service-level protocol branch.
func TestFunCloudMaterialServiceOpensSourceBeforeUpload(t *testing.T) {
	db := withAssetGroupPolicyDB(t)
	channel := createMoxingAssetPolicyChannel(t, db, "https://upstream.example")
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudMaterial, AssetMinURLTTLSeconds: 1})
	require.NoError(t, db.Save(channel).Error)
	calls := 0
	withAssetPolicyHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("unexpected provider request")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateRemoteAsset(ctx, "default", 1, generalAssetPolicyRequest("explicit-group"))
	require.ErrorIs(t, err, ErrInvalidAssetRequest)
	assert.Zero(t, calls)
}
