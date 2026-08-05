package model

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type videoOpenAPISchema struct {
	Required             []string                            `json:"required"`
	Properties           map[string]videoOpenAPISchema       `json:"properties"`
	Enum                 []string                            `json:"enum"`
	Minimum              *float64                            `json:"minimum"`
	Maximum              *float64                            `json:"maximum"`
	AdditionalProperties any                                 `json:"additionalProperties"`
	OneOf                []videoOpenAPISchema                `json:"oneOf"`
	ModelArkCapabilities []ModelArkVideoCapabilityProjection `json:"x-modelark-model-capabilities"`
}

type videoOpenAPIDocument struct {
	Components struct {
		Schemas map[string]videoOpenAPISchema `json:"schemas"`
	} `json:"components"`
	Paths map[string]struct {
		Post struct {
			RequestBody struct {
				Content map[string]struct {
					Schema videoOpenAPISchema `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"post"`
	} `json:"paths"`
}

func TestPublishedVideoSKURegistryMatchesOpenAPIAndUserGuides(t *testing.T) {
	seedanceGuide, err := os.ReadFile("../docs/30-engineering/视频模型API用户调用指南.md")
	require.NoError(t, err)
	openAPI, err := os.ReadFile("../docs/openapi/relay.json")
	require.NoError(t, err)
	var specification videoOpenAPIDocument
	require.NoError(t, common.Unmarshal(openAPI, &specification))
	modelArkSchema := specification.Components.Schemas["ModelArkVideoCreateRequest"]
	assert.Equal(t, PublicModelArkVideoCapabilityProjection(), modelArkSchema.ModelArkCapabilities)
	for _, projection := range modelArkSchema.ModelArkCapabilities {
		assert.NotContains(t, projection.PublicModel, "-720p", "Feicai v2 must remain absent from public OpenAPI until release evidence closes")
		assert.NotContains(t, projection.PublicModel, "-1080p", "Feicai v2 must remain absent from public OpenAPI until release evidence closes")
		assert.NotContains(t, projection.PublicModel, "-4k", "Feicai v2 must remain absent from public OpenAPI until release evidence closes")
	}

	for _, publicModel := range []string{
		VideoSKUSeedanceBytePlus,
		VideoSKUSeedance20Oversea,
		VideoSKUDoubaoSeedance20260128,
		VideoSKUSeedance20Standard,
		VideoSKUSeedance20Fast,
	} {
		capability, ok := ResolveVideoSKUCapability(publicModel)
		require.True(t, ok, publicModel)
		assert.Equal(t, string(dto.VideoContractModelArkV3), capability.ContractID, publicModel)
		assert.Contains(t, string(seedanceGuide), "`"+publicModel+"`", publicModel)
		assert.Contains(t, string(openAPI), publicModel, publicModel)
		assert.ElementsMatch(t, modelArkSchema.Required, capability.RequiredFields, publicModel)
		for _, field := range capability.RequestFields {
			assert.Contains(t, modelArkSchema.Properties, field, publicModel)
		}
		classifiedFields := make(map[string]struct{}, len(capability.RequestFields)+len(capability.UnsupportedFields))
		for _, field := range capability.RequestFields {
			classifiedFields[field] = struct{}{}
		}
		for _, field := range capability.UnsupportedFields {
			classifiedFields[field] = struct{}{}
		}
		for field := range modelArkSchema.Properties {
			assert.Contains(t, classifiedFields, field, publicModel)
		}
		implementations := LinkImplementationsForSKU(publicModel)
		require.NotEmpty(t, implementations, publicModel)
		for _, implementation := range implementations {
			implemented, found := ResolveVideoSKUImplementationCapability(publicModel, dto.LinkImplementationRef{ID: implementation.ID, Version: implementation.Version})
			require.True(t, found, publicModel)
			require.True(t, VideoSKUCapabilitiesEquivalent(capability, implemented), publicModel)
		}
	}

	klingGuide, err := os.ReadFile("../web/public/docs-content/zh/api-reference/videos/kling.md")
	require.NoError(t, err)
	for _, publicModel := range []string{VideoSKUKlingV1, VideoSKUKlingV16, VideoSKUKlingV2Master} {
		capability, ok := ResolveVideoSKUCapability(publicModel)
		require.True(t, ok, publicModel)
		assert.Equal(t, string(dto.VideoContractKlingV1), capability.ContractID, publicModel)
		assert.Equal(t, []int{constant.ChannelTypeKling}, capability.RequiredChannelTypes, publicModel)
		assert.Contains(t, string(klingGuide), "`"+publicModel+"`", publicModel)
		assert.Contains(t, string(openAPI), publicModel, publicModel)
		klingSchema := specification.Components.Schemas["KlingVideoRequest"]
		klingFields := make([]string, 0, len(klingSchema.Properties))
		for field := range klingSchema.Properties {
			klingFields = append(klingFields, field)
		}
		assert.ElementsMatch(t, capability.RequestFields, klingFields, publicModel)
		assert.ElementsMatch(t, capability.RequiredFields, klingSchema.Required, publicModel)
		assert.ElementsMatch(t, capability.Modes, klingSchema.Properties["mode"].Enum, publicModel)
		assert.ElementsMatch(t, capability.Ratios, klingSchema.Properties["aspect_ratio"].Enum, publicModel)
		durationValues := make([]string, 0, len(capability.DurationValues))
		for _, duration := range capability.DurationValues {
			durationValues = append(durationValues, strconv.Itoa(duration))
		}
		assert.ElementsMatch(t, durationValues, klingSchema.Properties["duration"].Enum, publicModel)
		require.NotNil(t, klingSchema.Properties["cfg_scale"].Minimum)
		require.NotNil(t, klingSchema.Properties["cfg_scale"].Maximum)
		assert.Equal(t, capability.MinCFGScale, *klingSchema.Properties["cfg_scale"].Minimum)
		assert.Equal(t, capability.MaxCFGScale, *klingSchema.Properties["cfg_scale"].Maximum)
		for _, field := range capability.RequestFields {
			assert.Contains(t, string(klingGuide), "`"+field+"`", field)
		}
		implemented, found := ResolveVideoSKUImplementationCapability(publicModel, dto.LinkImplementationRef{ID: LinkImplementationKlingVideos, Version: LinkImplementationVersionV1})
		require.True(t, found)
		require.True(t, VideoSKUCapabilitiesEquivalent(capability, implemented))
	}
	assert.Contains(t, string(klingGuide), "GET /v1/models")
	assert.Contains(t, string(openAPI), "createKlingText2Video")
	assert.Contains(t, string(openAPI), "createKlingImage2Video")

	jimengGuide, err := os.ReadFile("../web/public/docs-content/zh/api-reference/videos/jimeng.md")
	require.NoError(t, err)
	jimeng, ok := ResolveVideoSKUCapability(VideoSKUJimengVGFMT2VL20)
	require.True(t, ok)
	assert.Equal(t, string(dto.VideoContractJimeng), jimeng.ContractID)
	assert.Equal(t, []int{constant.ChannelTypeJimeng}, jimeng.RequiredChannelTypes)
	assert.True(t, strings.Contains(string(jimengGuide), "GET /v1/models"))
	assert.Contains(t, string(jimengGuide), "`"+VideoSKUJimengVGFMT2VL20+"`")
	assert.Contains(t, string(openAPI), VideoSKUJimengVGFMT2VL20)
	assert.Contains(t, string(openAPI), "createJimengVideo")
	jimengSchema := specification.Paths["/jimeng/"].Post.RequestBody.Content["application/json"].Schema
	jimengSchemaFields := make([]string, 0, len(jimengSchema.Properties))
	for field := range jimengSchema.Properties {
		jimengSchemaFields = append(jimengSchemaFields, field)
	}
	assert.ElementsMatch(t, append(jimeng.RequestFields, "task_id"), jimengSchemaFields)
	additionalProperties, ok := jimengSchema.AdditionalProperties.(bool)
	require.True(t, ok)
	assert.False(t, additionalProperties)
	require.Len(t, jimengSchema.OneOf, 2)
	assert.ElementsMatch(t, []string{"req_key"}, jimengSchema.OneOf[0].Required)
	assert.ElementsMatch(t, []string{"task_id"}, jimengSchema.OneOf[1].Required)
	for _, field := range jimeng.RequestFields {
		assert.Contains(t, jimengSchema.Properties, field)
		assert.Contains(t, string(jimengGuide), "`"+field+"`", field)
	}
	implemented, found := ResolveVideoSKUImplementationCapability(jimeng.PublicModel, dto.LinkImplementationRef{ID: LinkImplementationJimengVideos, Version: LinkImplementationVersionV1})
	require.True(t, found)
	require.True(t, VideoSKUCapabilitiesEquivalent(jimeng, implemented))
	assert.Contains(t, string(klingGuide), "不发布列表、平台内容代理、取消或删除")
	assert.Contains(t, string(jimengGuide), "不发布列表、平台内容代理、取消或删除")
}
