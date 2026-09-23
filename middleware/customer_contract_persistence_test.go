package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestContractRejectionsPersistThroughDistribute(t *testing.T) {
	for _, scenario := range []struct {
		name, reason string
		status       int
	}{
		{"token_limit", "token_model_forbidden", 403},
		{"not_listed", "contract_model_not_listed", 403},
		{"disabled", "contract_candidates_unavailable", 503},
		{"capability_missing", "contract_candidates_unavailable", 503},
		{"read_failure", "contract_route_lookup_failed", 503},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, user, contract := setupCustomerContractMiddlewareDB(t)
			require.NoError(t, model.LOG_DB.AutoMigrate(&model.ErrorEvent{}))
			if scenario.name == "disabled" {
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("status", common.ChannelStatusAutoDisabled).Error)
			}
			if scenario.name == "capability_missing" {
				require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", contract.Rules[0].ChannelId).Update("enabled", false).Error)
			}
			if scenario.name == "read_failure" {
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("diag:fail_channel_read", func(tx *gorm.DB) {
					if tx.Statement.Table == "channels" {
						tx.AddError(errors.New("injected read failure"))
					}
				}))
			}
			publicModel := "Model-A"
			if scenario.name == "not_listed" {
				publicModel = "Unlisted"
			}
			sink := logtest.New(t)
			engine := gin.New()
			engine.Use(sink.Middleware(), func(c *gin.Context) {
				c.Set(common.RequestIdKey, "reject-"+scenario.name)
				c.Set("route_tag", "relay")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
				common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				if scenario.name == "token_limit" {
					common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
					common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{})
				}
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			})
			engine.POST("/v1/chat/completions", Distribute(), func(c *gin.Context) { t.Error("rejected request reached relay"); c.Status(200) })
			// Model diagnostics must not depend on capturing a large customer body.
			body := `{"model":"` + publicModel + `","messages":[{"role":"user","content":"` + strings.Repeat("x", 70*1024) + `"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			assert.Equal(t, scenario.status, response.Code)
			assert.NotContains(t, response.Body.String(), "contract-route-channel")
			assert.NotContains(t, response.Body.String(), "contract_version")
			require.Eventually(t, func() bool { return sink.Health().Persisted == 1 }, time.Second, time.Millisecond)
			assert.EqualValues(t, 1, sink.Health().Accepted)
			rows, total, err := model.GetErrorEvents(model.ErrorEventFilter{RequestId: "reject-" + scenario.name}, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, rows, 1)
			assert.Equal(t, publicModel, rows[0].ModelName)
			assert.Equal(t, scenario.reason, rows[0].Reason)
			assert.Equal(t, scenario.status, rows[0].Status)
			var detail map[string]string
			require.NoError(t, common.UnmarshalJsonStr(rows[0].Detail, &detail))
			assert.Equal(t, strconv.Itoa(contract.Id), detail["contract_id"])
			assert.Equal(t, strconv.FormatInt(contract.Version, 10), detail["contract_version"])
			if scenario.name == "capability_missing" {
				assert.Equal(t, "capability_missing=1", detail["candidates_blocked"])
			}
		})
	}
}

func TestContractTypedMismatchWithoutPin(t *testing.T) {
	_, user, contract := setupCustomerContractMiddlewareDB(t)
	sink := logtest.New(t)
	engine := gin.New()
	engine.POST("/v1/batches", sink.Middleware(), func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
		common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
		_, err := service.ResolveCustomerContractRequest(c, "Model-A")
		require.NoError(t, err)
		for _, pin := range []int{0, contract.Rules[0].ChannelId} {
			_, _, err = service.CustomerContractTypedChannel(c, "Model-A", constant.ChannelTypeAzureBatch, pin)
			require.ErrorIs(t, err, service.ErrCustomerContractScope)
		}
		report, ok := clienterrlog.PeekReport(c.Request.Context())
		require.True(t, ok)
		assert.Equal(t, "contract_channel_mismatch", report.Reason)
		assert.Equal(t, strconv.FormatInt(contract.Version, 10), report.Detail["contract_version"])
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/batches", nil))
}
