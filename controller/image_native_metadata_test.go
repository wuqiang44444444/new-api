package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestNativeImageMetadataPreservesAzureDeploymentAndCustomerAlias(t *testing.T) {
	for _, tc := range []struct {
		name, customer, provider string
		channelType              int
	}{
		{"azure-deployment", "gpt-image-2", "private-deploy.v2", constant.ChannelTypeAzure},
		{"openai-alias", "customer-picture", "gpt-image-2", constant.ChannelTypeOpenAI},
		{"openai-image-25-alias", "customer-picture", "gpt-image-2.5-flare", constant.ChannelTypeOpenAI},
		{"openai-image-25-snapshot", "customer-picture", "gpt-image-2.5-sunburst-2026-09-08", constant.ChannelTypeOpenAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupModelListControllerTestDB(t)
			mapping, err := common.Marshal(map[string]string{tc.customer: tc.provider})
			require.NoError(t, err)
			channel := &model.Channel{Id: 804, Type: tc.channelType, Name: "native-metadata", Key: "fixture", Group: "default", Models: tc.customer, Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(string(mapping))}
			require.NoError(t, channel.Insert())
			apis, err := model.GetPublicMediaModelAPIs([]string{tc.customer}, []string{"default"})
			require.NoError(t, err)
			require.NotNil(t, apis[tc.customer])
			api := apis[tc.customer].Image
			require.NotNil(t, api)
			require.NotNil(t, api.Async)
			assert.False(t, api.Async.StreamPriority)
			assert.Equal(t, "/v1/tasks/{task_id}", api.Async.QueryPath)
			require.NotNil(t, api.Edit)
			assert.Equal(t, "/v1/images/edits", api.Edit.Path)
			assert.Equal(t, tc.customer, api.Edit.Model)
			assert.Equal(t, "application/json", api.Edit.ContentType)
			assert.Equal(t, []string{"model", "prompt", "images"}, api.Edit.RequiredFields)
			encoded, err := common.Marshal(api)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), tc.provider)
		})
	}
}

func TestNativeImageMetadataKeepsCommonCapabilitiesAcrossChannels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		types []int
		async bool
	}{
		{"openai_custom", []int{constant.ChannelTypeOpenAI, constant.ChannelTypeCustom}, false},
		{"custom_azure", []int{constant.ChannelTypeCustom, constant.ChannelTypeAzure}, false},
		{"azure_advanced_custom", []int{constant.ChannelTypeAzure, constant.ChannelTypeAdvancedCustom}, false},
		{"openai_azure", []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure}, true},
		{"three_channels", []int{constant.ChannelTypeOpenAI, constant.ChannelTypeCustom, constant.ChannelTypeAzure}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSelfUseModeEnabled(t)
			setupModelListControllerTestDB(t)
			for i, channelType := range tc.types {
				channel := &model.Channel{Id: 810 + i, Type: channelType, Name: "mixed-image-" + strconv.Itoa(i), Key: "fixture", Group: "default", Models: "gpt-image-2", Status: common.ChannelStatusEnabled}
				require.NoError(t, channel.Insert())
			}
			model.InvalidatePricingCache()
			model.GetPricing()
			model.InitChannelCache()
			want := publicmodel.NativeImageAPI("gpt-image-2")
			if tc.async {
				want = publicmodel.NativeAsyncImageAPI("gpt-image-2", "gpt-image-2")
			}
			for _, list := range []bool{false, true} {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				if list {
					ListModels(c, constant.ChannelTypeOpenAI)
				} else {
					c.Params = gin.Params{{Key: "model", Value: "gpt-image-2"}}
					RetrieveModel(c, constant.ChannelTypeOpenAI)
				}
				require.Equal(t, http.StatusOK, recorder.Code)
				var response dto.OpenAIModels
				if list {
					var result struct {
						Data []dto.OpenAIModels `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
					require.Len(t, result.Data, 1)
					response = result.Data[0]
				} else {
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				}
				require.NotNil(t, response.API)
				// Compare wire contracts: interface numeric values round-trip as float64.
				encoded, err := common.Marshal(want)
				require.NoError(t, err)
				actual, err := common.Marshal(response.API)
				require.NoError(t, err)
				assert.JSONEq(t, string(encoded), string(actual))
			}
		})
	}
}
