package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientErrorRecorderAcrossActualPluginDispatcher(t *testing.T) {
	setupRelayRouterTestDB(t)
	// 本测试只验证 WARN 出口与请求行为；显式关闭全局持久化钩子，
	// 避免事件落入测试 SQLite（其中没有 error_events 表）。
	clienterrlog.SetEventPersister(nil)
	t.Cleanup(func() { clienterrlog.SetEventPersister(nil) })
	user := model.User{Username: "plugin-client-error", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "clienterrorpluginfixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	plugin := compileRouterPlugin(t, "client-error", "1.0.0", `[{method:"GET",path:"/vendor/diagnostic/:task_id",type:"query",render:"native"}]`)
	for _, status := range []int{400, 403, 404, 429, 200, 502} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			capture := logtest.New(t)
			handlerCalls := 0
			handlers := func(g *jsplugin.RoutingGeneration, b jsplugin.RouteBinding) []gin.HandlerFunc {
				pipeline := productionPluginRouteHandlers(g, b)
				// Keep real pinning and TokenAuth; replace provider execution with a fixed result.
				return []gin.HandlerFunc{pipeline[0], pipeline[1], func(c *gin.Context) { handlerCalls++; c.String(status, "original response") }}
			}
			outer, registry := newPluginRouterTest(t, []*jsplugin.LoadedPlugin{plugin}, handlers)
			outer.Use(capture.Middleware())
			outer.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)
			req := httptest.NewRequest("GET", "/vendor/diagnostic/private-task?private=value", nil)
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			response := httptest.NewRecorder()
			outer.ServeHTTP(response, req)
			require.Equal(t, status, response.Code)
			assert.Equal(t, "original response", response.Body.String())
			assert.Equal(t, 1, handlerCalls)
			if status >= 400 && status < 500 {
				log := capture.String()
				assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"))
				assert.Contains(t, log, "route=/vendor/diagnostic/:task_id")
				assert.Contains(t, log, "module=relay")
				assert.Contains(t, log, "user_id="+strconv.Itoa(user.Id))
				assert.Contains(t, log, "request_id=request-phase-two")
				assert.NotContains(t, log, "private-task")
				assert.NotContains(t, log, token.Key)
			} else {
				// 200：不产生事件；502（relay 模块）只走持久化出口，不写 WARN 行。
				expectedAccepted := map[int]uint64{200: 0, 502: 1}[status]
				assert.Equal(t, expectedAccepted, capture.Health().Accepted)
				assert.Zero(t, capture.Health().Written)
			}
			before := capture.Health().Accepted
			rejected := httptest.NewRecorder()
			outer.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/vendor/diagnostic/private-task", nil))
			assert.Equal(t, 401, rejected.Code)
			assert.Equal(t, before, capture.Health().Accepted)
			assert.Equal(t, 1, handlerCalls)
		})
	}
}

func TestClientErrorHealthEndpointRequiresRoot(t *testing.T) {
	setupRelayRouterTestDB(t)
	for _, role := range []int{common.RoleCommonUser, common.RoleRootUser} {
		pat := "health-fixture-" + strconv.Itoa(role)
		user := model.User{Username: "health-user-" + strconv.Itoa(role), Status: common.UserStatusEnabled, Role: role, Group: "default", AccessToken: &pat, AuthVersion: 1, AffCode: "health-aff-" + strconv.Itoa(role)}
		require.NoError(t, model.DB.Create(&user).Error)
	}
	engine := gin.New()
	SetApiRouter(engine)
	for _, tc := range []struct {
		pat    string
		status int
	}{{"", 401}, {"health-fixture-1", 403}, {"health-fixture-100", 200}} {
		req := httptest.NewRequest("GET", "/api/system-info/client-error-log", nil)
		if tc.pat != "" {
			req.Header.Set("Authorization", "Bearer "+tc.pat)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		require.Equal(t, tc.status, response.Code)
		if tc.status == 200 {
			assert.Contains(t, response.Body.String(), `"dropped":`)
			assert.Contains(t, response.Body.String(), `"write_started_at":`)
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
		}
	}
}
