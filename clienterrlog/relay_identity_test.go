package clienterrlog

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRelayErrorIdentityFallback(t *testing.T) {
	for _, tc := range []struct {
		name, module, stage, reason                string
		contextChannel, reportChannel, wantChannel int
		contextModel, reportModel, wantModel       string
	}{
		{name: "native final channel", module: "relay", contextChannel: 155, wantChannel: 155, contextModel: "customer-image", wantModel: "customer-image"},
		{name: "explicit report wins", module: "relay", contextChannel: 155, reportChannel: 7, wantChannel: 7, contextModel: "customer-image", reportModel: "explicit-model", wantModel: "explicit-model"},
		{name: "selection rejection", module: "relay", stage: "channel_selection", reason: "contract_candidates_unavailable"},
		{name: "selection rejection after an earlier attempt", module: "relay", stage: "channel_selection", reason: "channel_unavailable", contextChannel: 149},
		{name: "model rejection", module: "relay", stage: "model_access", reason: "token_model_forbidden", reportModel: "forbidden-model", wantModel: "forbidden-model"},
		{name: "gateway rate limit", module: "relay"},
		{name: "other module unchanged", module: "asset", contextChannel: 155, contextModel: "customer-image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
			c.Set("original_model", tc.contextModel)
			c.Set("channel_id", tc.contextChannel)
			report := &requestReport{model: tc.reportModel, channelID: tc.reportChannel, stage: tc.stage, reason: tc.reason}
			event := buildClientErrorEvent(c, time.Now(), 429, authPassMarker{Module: tc.module}, report)
			assert.Equal(t, tc.wantChannel, event.ChannelID)
			assert.Equal(t, tc.wantModel, event.Model)
			if tc.reason != "" {
				assert.Equal(t, tc.reason, event.Reason)
				assert.Equal(t, tc.stage, event.Stage)
			}
		})
	}
}
