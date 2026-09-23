package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// runContractRejectGate drives one contract distribute-gate request and returns
// the merged diagnostics captured inside the handler plus the abort status.
func runContractRejectGate(t *testing.T, fixture *testDBFixture, publicModel string, mutate func(*gin.Context), postGate func(*gin.Context)) (clienterrlog.Report, int) {
	t.Helper()
	user, contract := fixture.user, fixture.contract
	var report clienterrlog.Report
	status := http.StatusOK
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", logtest.New(t).Middleware(), func(c *gin.Context) {
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
		common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
		if mutate != nil {
			mutate(c)
		}
		if applyCustomerContractDistributeGate(c, publicModel, true) {
			status = c.Writer.Status()
		}
		if postGate != nil {
			postGate(c)
		}
		report, _ = clienterrlog.PeekReport(c.Request.Context())
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	return report, status
}

type testDBFixture struct {
	user     model.User
	contract model.ContractEntitySnapshot
}

// Each contract rejection branch surfaces a fixed stage, a fixed reason and the
// resolved customer model in the unified event, while public status semantics
// stay untouched: authorization rejections 403, availability failures 503.
func TestContractRejectionDiagnosticsPerBranch(t *testing.T) {
	db, user, contract := setupCustomerContractMiddlewareDB(t)
	fixture := &testDBFixture{user: user, contract: contract}

	t.Run("token_model_limit_keeps_403", func(t *testing.T) {
		report, status := runContractRejectGate(t, fixture, "Model-A", func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"Other-Model": true})
		}, nil)
		require.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, "model_access", report.Stage)
		assert.Equal(t, "token_model_forbidden", report.Reason)
		assert.Equal(t, "Model-A", report.Model)
	})

	t.Run("model_not_listed_keeps_403_with_contract_identity", func(t *testing.T) {
		report, status := runContractRejectGate(t, fixture, "Unlisted-Model", nil, nil)
		require.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, "contract_scope", report.Stage)
		assert.Equal(t, "contract_model_not_listed", report.Reason)
		assert.Equal(t, "Unlisted-Model", report.Model)
		assert.Equal(t, strconv.Itoa(contract.Id), report.Detail["contract_id"])
		assert.NotEmpty(t, report.Detail["contract_version"])
	})

	t.Run("candidates_unavailable_reports_blocked_category", func(t *testing.T) {
		var report clienterrlog.Report
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		engine.POST("/v1/chat/completions", logtest.New(t).Middleware(), func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
			common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
			common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("status", common.ChannelStatusAutoDisabled).Error)
			model.InitChannelCache()
			_, err := service.ResolveCustomerContractRequest(c, "Model-A")
			require.NoError(t, err)
			_, _, err = service.SelectCustomerContractChannel(&service.RetryParam{Ctx: c, ModelName: "Model-A", TokenGroup: "default"}, true)
			require.ErrorIs(t, err, service.ErrCustomerContractScope)
			report, _ = clienterrlog.PeekReport(c.Request.Context())
			c.Status(http.StatusServiceUnavailable)
		})
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
		assert.Equal(t, "channel_selection", report.Stage)
		assert.Equal(t, "contract_candidates_unavailable", report.Reason)
		assert.Equal(t, "Model-A", report.Model)
		assert.Equal(t, "1", report.Detail["candidates_total"])
		assert.Contains(t, report.Detail["candidates_blocked"], "channel_disabled=1")
	})

	t.Run("route_lookup_failure_reports_dedicated_reason", func(t *testing.T) {
		var report clienterrlog.Report
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		failSourceRead := true
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:contract_diag_fail_read", func(tx *gorm.DB) {
			if failSourceRead {
				tx.AddError(errors.New("source read unavailable"))
			}
		}))
		t.Cleanup(func() { failSourceRead = false })
		engine.POST("/v1/chat/completions", logtest.New(t).Middleware(), func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
			common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
			common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
			_, err := service.ResolveCustomerContractRequest(c, "Model-A")
			require.NoError(t, err)
			_, _, err = service.SelectCustomerContractChannel(&service.RetryParam{Ctx: c, ModelName: "Model-A", TokenGroup: "default"}, true)
			require.Error(t, err)
			require.NotErrorIs(t, err, service.ErrCustomerContractScope)
			report, _ = clienterrlog.PeekReport(c.Request.Context())
			c.Status(http.StatusServiceUnavailable)
		})
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
		assert.Equal(t, "channel_selection", report.Stage)
		assert.Equal(t, "contract_route_lookup_failed", report.Reason)
	})

	t.Run("channel_not_in_scope_reports_locked_conflict", func(t *testing.T) {
		report, _ := runContractRejectGate(t, fixture, "Model-A", nil, func(c *gin.Context) {
			// 恢复候选子测试停用的渠道，使本子测试只覆盖“渠道不在合同路由内”。
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("status", common.ChannelStatusEnabled).Error)
			common.SetContextKey(c, constant.ContextKeyChannelId, 424242)
			require.ErrorIs(t, service.ValidateCustomerContractChannel(c, "Model-A", 424242), service.ErrCustomerContractScope)
		})
		assert.Equal(t, "channel_selection", report.Stage)
		assert.Equal(t, "contract_channel_not_in_scope", report.Reason)
		assert.Equal(t, "Model-A", report.Model)
	})

	t.Run("contract_unavailable_keeps_503", func(t *testing.T) {
		service.ResetContractEntityCacheForTest()
		require.NoError(t, db.Where("id = ?", contract.Id).Delete(&model.CustomerContract{}).Error)
		report, status := runContractRejectGate(t, fixture, "Model-A", nil, nil)
		require.Equal(t, http.StatusServiceUnavailable, status)
		assert.Equal(t, "contract_scope", report.Stage)
		assert.Equal(t, "contract_unavailable", report.Reason)
		assert.Equal(t, "Model-A", report.Model)
	})
}
