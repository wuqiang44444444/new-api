package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduModelsAvailableInAdminContractAndTemplateSelectors(t *testing.T) {
	admin, user, existing := setupCustomerContractControllerDB(t)
	var allModels []string
	for _, region := range []string{"cn", "global"} {
		var models []string
		mapping := map[string]string{}
		for _, variant := range []struct{ alias, provider string }{
			{"2-0", "viduq3-drama-std"}, {"2-0-fast", "viduq3-drama-fast"},
			{"2-0-mini", "viduq3-drama-mini"}, {"2-5", "viduq3.1-drama-std"},
		} {
			alias := fmt.Sprintf("seedance-%s-vidu-%s", variant.alias, region)
			provider := variant.provider
			if region == "global" {
				provider = strings.Replace(provider, "-drama-", "-drama-ab-", 1)
			}
			models = append(models, alias)
			mapping[alias] = provider
		}
		encoded, err := common.Marshal(mapping)
		require.NoError(t, err)
		channel := model.Channel{Name: "Vidu " + region, Type: constant.ChannelTypeSeedanceLink, Models: strings.Join(models, ","), Group: "default", Status: common.ChannelStatusEnabled, Key: "fixture-private-key", ModelMapping: common.GetPointer(string(encoded))}
		channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
		require.NoError(t, model.DB.Create(&channel).Error)
		allModels = append(allModels, models...)
		var abilities int64
		require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Count(&abilities).Error)
		assert.Zero(t, abilities)
	}
	for _, handler := range []struct {
		name string
		run  func(*gin.Context)
	}{
		{"catalog", GetCustomerContractCatalog}, {"templates", GetCustomerContractTemplateOptions},
	} {
		t.Run(handler.name, func(t *testing.T) {
			c, recorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract/catalog", "", admin, user)
			handler.run(c)
			assert.Equal(t, http.StatusOK, recorder.Code)
			for _, alias := range allModels {
				assert.Contains(t, recorder.Body.String(), `"`+alias+`"`)
			}
			assert.NotContains(t, recorder.Body.String(), "fixture-private-key")
			assert.NotContains(t, recorder.Body.String(), "viduq3")
		})
	}
	unchanged, err := model.GetContractEntitySnapshot(existing.Id, true)
	require.NoError(t, err)
	assert.Equal(t, existing.Version, unchanged.Version)
	assert.Equal(t, existing.Rules, unchanged.Rules, "discovery never adds models to an existing contract")
}
