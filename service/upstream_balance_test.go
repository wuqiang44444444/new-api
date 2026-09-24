package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBalanceProviderContracts(t *testing.T) {
	tests := []struct {
		name, kind                             string
		token                                  bool
		responses                              map[string]string
		status, reason, amount, unit, category string
	}{
		{name: "leone preserves precise negative balance and currency", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `{"code":0,"data":{"balance":"-0.123456789012345678","currency":"CNY","frozen_balance":10}}`}, status: "ok", amount: "-0.123456789012345678", unit: "CNY"},
		{name: "leone accepts real zero", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `{"code":0,"data":{"balance":0,"currency":"USD"}}`}, status: "ok", amount: "0", unit: "USD"},
		{name: "business error cannot become zero", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `{"code":1,"data":{"balance":0,"currency":"USD"}}`}, status: "error", reason: "invalid_response"},
		{name: "missing amount cannot become zero", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `{"code":0,"data":{"currency":"USD"}}`}, status: "error", reason: "invalid_response"},
		{name: "HTML cannot become zero", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `<html>login</html>`}, status: "error", reason: "invalid_response"},
		{name: "NewAPI explicit unlimited is not money", kind: "newapi", token: true, responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":100000000}`, "/api/usage/token/": `{"code":true,"data":{"object":"token_usage","unlimited_quota":true}}`}, status: "unlimited", reason: "unlimited_key"},
		{name: "sentinel without proof is not unlimited", kind: "newapi", responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":100000000}`}, status: "unavailable", reason: "account_balance_unavailable"},
		{name: "finite compatible amount uses upstream currency without exchange", kind: "newapi", token: true, responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":5}`, "/dashboard/billing/usage": `{"object":"list","total_usage":600.000000000000000001}`, "/api/status": `{"success":true,"data":{"quota_display_type":"CNY","usd_exchange_rate":7}}`}, status: "ok", amount: "-1.00000000000000000001", unit: "CNY"},
		{name: "absent metadata never implies USD", kind: "newapi", responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":5}`, "/dashboard/billing/usage": `{"object":"list","total_usage":0}`, "/api/status": `{"success":false}`}, status: "ok", amount: "5"},
		{name: "wrong usage object is rejected", kind: "newapi", responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":5}`, "/dashboard/billing/usage": `{"total_usage":0}`}, status: "error", reason: "invalid_response"},
		{name: "QHAIGC uses its verified direct field with unknown unit", kind: "qhaigc", responses: map[string]string{"/dashboard/billing/subscription": `{"object":"billing_subscription","hard_limit_usd":8,"balance":1.25}`}, status: "ok", amount: "1.25"},
		{name: "Vidu keeps credits and does not add packages", kind: "vidu", responses: map[string]string{"/ent/v2/credits": `{"remains":[{"type":"metered","credit_remain":12}],"packages":[{"credit_remain":12}]}`}, status: "ok", amount: "12", unit: "credits", category: "metered"},
		{name: "missing Vidu credits cannot become zero", kind: "vidu", responses: map[string]string{"/ent/v2/credits": `{"remains":[{"type":"metered"}]}`}, status: "error", reason: "invalid_response"},
		{name: "huge exponent is rejected", kind: "leone", responses: map[string]string{"/api/v2/open/balance": `{"code":0,"data":{"balance":"1e999999999","currency":"USD"}}`}, status: "error", reason: "invalid_response"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seen := make(map[string]bool)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				auth := "Bearer test-credential"
				if test.kind == "vidu" {
					auth = "Token test-credential"
				}
				if r.URL.Path == "/api/status" {
					auth = ""
				}
				assert.Equal(t, auth, r.Header.Get("Authorization"))
				body, exists := test.responses[r.URL.Path]
				assert.True(t, exists, "unexpected path: %s", r.URL.Path)
				seen[r.URL.Path] = true
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			result := queryUpstreamBalanceProvider(context.Background(), server.Client(), upstreamBalancePlan{kind: test.kind, origin: server.URL, tokenUsage: test.token}, "test-credential")
			assert.Equal(t, test.status, result.Status)
			assert.Equal(t, test.reason, result.Reason)
			assert.Len(t, seen, len(test.responses))
			if test.status == "ok" {
				require.Len(t, result.Amounts, 1)
				assert.Equal(t, UpstreamBalanceAmount{Amount: test.amount, Unit: test.unit, Category: test.category}, result.Amounts[0])
			} else {
				assert.Empty(t, result.Amounts)
			}
		})
	}
}

func TestUpstreamBalanceErrorsAreSanitized(t *testing.T) {
	for _, status := range []int{301, 401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://untrusted.example/steal-key")
				w.WriteHeader(status)
				fmt.Fprint(w, "secret-provider-message-and-key")
			}))
			defer server.Close()
			client := server.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			result := queryUpstreamBalanceProvider(context.Background(), client, upstreamBalancePlan{kind: "leone", origin: server.URL}, "test-credential")
			assert.Equal(t, "error", result.Status)
			assert.NotEmpty(t, result.Reason)
			encoded, err := common.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "secret-provider")
			assert.NotContains(t, string(encoded), "test-credential")
			assert.Empty(t, result.Amounts)
		})
	}
}

func TestUpstreamBalanceInventoryAndConfigurationDrift(t *testing.T) {
	base := "https://www.amutes.com"
	key1, key2 := "secret-key-0001", "secret-key-0002"
	channels := []*model.Channel{
		{Id: 1, Name: "first", Type: 1, BaseURL: &base, Key: key1, Status: 1},
		{Id: 2, Name: "disabled", Type: 1, BaseURL: &base, Key: key1, Status: 2},
		{Id: 3, Name: "multiple", Type: 1, BaseURL: &base, Key: key1 + "\n" + key2, Status: 1, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{1: 2}}},
	}
	rows := UpstreamBalanceInventory(channels)
	require.Len(t, rows, 2)
	assert.Len(t, rows[0].Channels, 3)
	assert.False(t, rows[0].Channels[1].Enabled)
	assert.False(t, rows[1].Channels[0].Enabled)
	assert.Equal(t, 1, rows[1].KeyIndex)
	assert.True(t, rows[1].Queryable)
	encoded, err := common.Marshal(rows)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), key1)
	assert.NotContains(t, string(encoded), key2)
	channels[2].Key = key2 + "\n" + key1
	result := QueryUpstreamBalance(context.Background(), channels[2], 1, rows[1].ID)
	assert.Equal(t, "connection_changed", result.Reason)
	assert.Empty(t, result.Amounts)
}

func TestUpstreamBalanceRegistryDoesNotProbeUnknownOrigins(t *testing.T) {
	for _, base := range []string{"https://unknown.example", "https://www.amutes.com.evil.example", "https://www.amutes.com?key=private", "https://private@www.amutes.com", "http://www.amutes.com"} {
		channel := &model.Channel{Id: 1, Type: 1, BaseURL: &base, Key: "test-credential"}
		rows := UpstreamBalanceInventory([]*model.Channel{channel})
		require.Len(t, rows, 1)
		assert.False(t, rows[0].Queryable)
		result := QueryUpstreamBalance(context.Background(), channel, 0, rows[0].ID)
		assert.Equal(t, "unsupported", result.Status)
		assert.NotContains(t, rows[0].Origin, "private")
	}
	base := "https://mm.leonecloud.com"
	plan, _, reason := upstreamBalanceProvider(&model.Channel{Type: 1, BaseURL: &base})
	assert.Empty(t, reason)
	assert.Equal(t, "https://mm-internal-cn.leonecloud.com", plan.origin)
	base = "https://www.amutes.com"
	_, _, reason = upstreamBalanceProvider(&model.Channel{Type: constant.ChannelTypeAzure, BaseURL: &base})
	assert.Equal(t, "unsupported_credentials", reason)
}

func TestUpstreamBalanceMalformedSettingsCannotWriteChannel(t *testing.T) {
	base, setting := "https://www.amutes.com", "{malformed"
	channel := &model.Channel{Id: 1, Type: 1, BaseURL: &base, Key: "test-credential", Setting: &setting, Balance: 123, Status: 1}
	// DB is deliberately not initialized: the read-only parser must not call Save.
	rows := UpstreamBalanceInventory([]*model.Channel{channel})
	result := QueryUpstreamBalance(context.Background(), channel, 0, rows[0].ID)
	assert.Equal(t, "invalid_connection", result.Reason)
	assert.Equal(t, setting, *channel.Setting)
	assert.Equal(t, 123.0, channel.Balance)
	assert.Equal(t, 1, channel.Status)
}

func TestUpstreamBalanceRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat(" ", 256*1024+1)) }))
	defer server.Close()
	result := queryUpstreamBalanceProvider(context.Background(), server.Client(), upstreamBalancePlan{kind: "leone", origin: server.URL}, "key")
	assert.Equal(t, "invalid_response", result.Reason)
	assert.Empty(t, result.Amounts)
}
