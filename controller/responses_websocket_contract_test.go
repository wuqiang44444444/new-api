package controller

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesWebSocketEnforcesNewPolicyOnEstablishedConnection(t *testing.T) {
	require.NoError(t, i18n.Init())
	fixture := newResponsesWSBillingTest(t, `tier("request", fixed(0.002))`, func(*websocket.Conn, *http.Request) {
		assert.Fail(t, "a denied event must not open an upstream connection")
	})
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	require.NoError(t, fixture.client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"ws-billing","input":"hello"}`)))
	denied := readResponsesWSTestEvent(t, fixture.client)
	require.Equal(t, "error", denied["type"], "%v", denied)
	assert.Equal(t, "contract_required", denied["error"].(map[string]any)["code"])
	fixture.closeAndWait(t)
	assertResponsesWSAccounting(t, fixture, nil)
}

func TestResponsesWebSocketRechecksContractOnEstablishedConnection(t *testing.T) {
	require.NoError(t, i18n.Init())
	var sent atomic.Int64
	fixture := newResponsesWSBillingTest(t, `tier("request", fixed(0.002))`, func(ws *websocket.Conn, _ *http.Request) {
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
			event := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_%d","status":"completed","model":"ws-billing","output":[],"usage":{"input_tokens":1000,"output_tokens":10,"total_tokens":1010}}}`, sent.Add(1))
			if !assert.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(event))) {
				return
			}
		}
	})
	t.Cleanup(service.ResetContractEntityCacheForTest)
	const call = `{"type":"response.create","model":"ws-billing","input":"hello"}`
	create := func() map[string]any {
		t.Helper()
		require.NoError(t, fixture.client.WriteMessage(websocket.TextMessage, []byte(call)))
		return readResponsesWSTestEvent(t, fixture.client)
	}
	contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: fixture.user.Id, AdminUserId: fixture.user.Id, Name: "Socket contract", Enabled: true, Reason: "Test socket reuse",
		Rules: []model.CustomerContractEntityRuleInput{{PublicModel: "ws-billing", RouteGroup: "default", ChannelId: fixture.channel.Id, RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	fixture.token.ContractId = contract.Id
	require.NoError(t, fixture.token.Update())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	for range 2 {
		completed := create()
		require.Equal(t, "response.completed", completed["type"], "%v", completed)
	}

	replacement := model.Channel{Name: "other-contract-channel", Group: "default", Models: "ws-billing", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&replacement).Error)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: replacement.Id, Group: "default", Model: "ws-billing", Enabled: true}).Error)
	require.NoError(t, model.DB.Model(&model.CustomerContractEntityRule{}).Where("contract_id = ? AND channel_id = ?", contract.Id, fixture.channel.Id).Update("channel_id", replacement.Id).Error)
	service.ResetContractEntityCacheForTest()
	denied := create()
	require.Equal(t, "error", denied["type"], "%v", denied)
	assert.Equal(t, "model_not_found", denied["error"].(map[string]any)["code"])
	fixture.closeAndWait(t)
	assert.EqualValues(t, 2, sent.Load())
	assertResponsesWSAccounting(t, fixture, []int{800, 800})
}
