package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelMappedHelperResolvesOrdinaryMappingContract(t *testing.T) {
	tests := []struct {
		name          string
		mapping       string
		wantModel     string
		wantMapped    bool
		wantErrorText string
	}{
		{name: "no mapping", wantModel: "customer-model"},
		{name: "chain", mapping: `{"customer-model":"provider-alias","provider-alias":"provider-model"}`, wantModel: "provider-model", wantMapped: true},
		{name: "self mapping", mapping: `{"customer-model":"customer-model"}`, wantModel: "customer-model"},
		{name: "cycle", mapping: `{"customer-model":"provider-alias","provider-alias":"customer-model"}`, wantModel: "customer-model", wantErrorText: "model_mapping_contains_cycle"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Set("model_mapping", test.mapping)
			info := &relaycommon.RelayInfo{
				OriginModelName: "customer-model",
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "customer-model",
				},
			}
			request := &relaykitdto.GeneralOpenAIRequest{Model: "customer-model"}

			err := ModelMappedHelper(context, info, request)

			if test.wantErrorText != "" {
				require.EqualError(t, err, test.wantErrorText)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantModel, info.UpstreamModelName)
			assert.Equal(t, test.wantModel, request.Model)
			assert.Equal(t, test.wantMapped, info.IsModelMapped)
		})
	}
}

func TestModelMappedHelperValidatesPublishedLinkExecution(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))

	channel := model.Channel{
		Type:         constant.ChannelTypeDoubaoVideo,
		Models:       "customer-seedance",
		Group:        "default",
		Status:       common.ChannelStatusEnabled,
		ModelMapping: common.GetPointer(`{"customer-seedance":"seedance-2.0-vip-720p-azhw"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamCreatePath:        "/v1/videos",
		VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		LinkImplementation: dto.LinkImplementationRef{
			ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV1,
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("model_mapping", `{"customer-seedance":"seedance-2.0-vip-720p-azhw"}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "customer-seedance",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channel.Id, UpstreamModelName: "customer-seedance",
		},
		LinkPublicationSnapshot: relaycommon.LinkPublicationSnapshot{
			LinkContractNamespace:    model.LinkContractNamespaceDefault,
			LinkRouteFamily:          string(model.LinkRouteFamilyModelArkVideo),
			PublishedLinkContractSKU: model.VideoSKUSeedance20Standard720P,
			LinkPublicationVersion:   1,
		},
	}
	request := &relaykitdto.GeneralOpenAIRequest{Model: "customer-seedance"}

	require.NoError(t, ModelMappedHelper(context, info, request))
	assert.Equal(t, "seedance-2.0-vip-720p-azhw", request.Model)
}
