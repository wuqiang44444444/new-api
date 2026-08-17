package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAssetDistinguishesUnknownFromConfirmedUnsupportedModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	callCreate := func(modelName string) *httptest.ResponseRecorder {
		body := `{"name":"portrait","asset_kind":"general","media_type":"image","model":"` + modelName + `","source":{"type":"url","url":"https://1.1.1.1/portrait.png"}}`
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/assets", strings.NewReader(body))
		context.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
		CreateAsset(context)
		return recorder
	}

	unknown := callCreate("model-does-not-exist")
	assert.Equal(t, http.StatusNotFound, unknown.Code)
	assert.Contains(t, unknown.Body.String(), `"code":"model_not_found"`)

	unsupported := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled,
		Name: "unsupported assets", Key: "unused", Group: "default", Models: "seedance-no-assets",
	}
	unsupported.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	})
	require.NoError(t, db.Create(unsupported).Error)

	confirmedUnsupported := callCreate("seedance-no-assets")
	assert.Equal(t, http.StatusUnprocessableEntity, confirmedUnsupported.Code)
	assert.Contains(t, confirmedUnsupported.Body.String(), `"code":"unsupported_asset_operation"`)
}
