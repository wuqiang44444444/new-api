package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAndValidOpenAIImageRequestAppliesLinkContract(t *testing.T) {
	newContext := func(body string) *gin.Context {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
		context.Request.Header.Set("Content-Type", "application/json")
		return context
	}

	context := newContext(`{"model":"seedream-5-qihang","prompt":"edit","size":"2K","image":["https://cdn.example/1.png","https://cdn.example/2.png"]}`)
	_, err := GetAndValidOpenAIImageRequest(context, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	capability, ok := common.GetContextKeyType[model.ImageSKUCapability](context, constant.ContextKeyResolvedImageSKUCapability)
	require.True(t, ok)
	assert.Equal(t, "seedream-5-qihang", capability.PublicModel)

	context = newContext(`{"model":"seedream-5-qihang","prompt":"edit","size":"2K","quality":"hd"}`)
	_, err = GetAndValidOpenAIImageRequest(context, relayconstant.RelayModeImagesGenerations)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quality is not supported")

	context = newContext(`{"model":"seedream-5-qihang","prompt":"edit","size":"2K","n":0}`)
	_, err = GetAndValidOpenAIImageRequest(context, relayconstant.RelayModeImagesGenerations)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "n must be between 1 and 1")
}
