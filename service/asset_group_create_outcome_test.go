package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type groupCreateHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f groupCreateHTTPDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestUnknownGroupCreateOutcomeIsNotBlindlyRetried(t *testing.T) {
	truncate(t)
	seedUser(t, 935, 0)
	asset := model.Asset{UserID: 935, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusCreating}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 10, CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets", Status: model.AssetBindingStatusCreating}
	require.NoError(t, model.DB.Create(&binding).Error)
	var calls atomic.Int32
	adapter := assetadapter.NewJoyCreatorAdapter("https://provider.example", "key", groupCreateHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":"timeout"}`))}, nil
	}))

	_, err := ensureAssetGroup(context.Background(), &asset, &binding, adapter)
	require.ErrorIs(t, err, errAssetGroupCreateOutcomeUnknown)
	_, err = ensureAssetGroup(context.Background(), &asset, &binding, adapter)
	require.ErrorIs(t, err, errAssetGroupCreateOutcomeUnknown)
	assert.Equal(t, int32(1), calls.Load())

	var group model.AssetGroupBinding
	require.NoError(t, model.DB.Where("user_id = ? AND group_kind = ?", asset.UserID, "general_aigc").First(&group).Error)
	assert.Equal(t, model.AssetBindingStatusCreateUnknown, group.Status)
}
