package relay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudModelArkExcessImagesRejectedBeforePricingAndHold(t *testing.T) {
	saveBillingConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"customer":"tiered_expr"}`, "billing_setting.billing_expr": `{}`}))
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()
	request := &dto.ModelArkVideoCreateRequest{Model: "customer", Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("A landscape")}}}
	for i := 0; i < 31; i++ {
		request.Content = append(request.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "asset://existing-image"}})
	}
	raw, err := common.Marshal(request)
	require.NoError(t, err)
	c, info := newTaskSubmitContext(t, "customer", `{"customer":"seedance-2-0"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeSeedanceLink)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, provider.URL)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3})
	common.SetContextKey(c, constant.ContextKeyTaskPromptValidated, true)
	common.SetContextKey(c, constant.ContextKeyTaskDurationValidated, true)
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: request})
	info.OriginModelName = "customer"
	// An absent price deliberately proves validation precedes price lookup.
	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_video_parameter", taskErr.Code)
	assert.Zero(t, common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	assert.Nil(t, info.Billing)
	assert.Nil(t, info.TieredBillingSnapshot)
	assert.Zero(t, calls)
}
