package seedance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceAdaptorRequiresModelArkV3Contract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"seedance-model","prompt":"hello"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_contract", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestSeedanceAdaptorAcceptsModelArkV3Contract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"seedance-model","prompt":"hello"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   &dto.ModelArkVideoCreateRequest{Model: "seedance-model"},
	})
	context.Set(string(constant.ContextKeyTaskPromptValidated), true)

	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.Nil(t, taskErr)
}
