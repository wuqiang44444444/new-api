package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractUnavailableReasonIsAdminOnly(t *testing.T) {
	for _, reason := range []string{"channel_disabled", "channel_missing", "capability_missing", "route_group_invalid"} {
		t.Run(reason, func(t *testing.T) {
			admin, user, contract := setupCustomerContractControllerDB(t)
			rule := contract.Rules[0]
			template, err := model.CreateCustomerContractTemplate(model.CreateCustomerContractTemplateParams{
				AdminUserId: admin.Id, Name: "Source diagnostics", Enabled: true, Reason: "test source",
				Rules: []model.CustomerContractEntityRuleInput{{PublicModel: rule.PublicModel, ChannelId: rule.ChannelId, RouteGroup: rule.RouteGroup, RatioUnits: rule.RatioUnits}},
			})
			require.NoError(t, err)
			switch reason {
			case "channel_disabled":
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", rule.ChannelId).Update("status", common.ChannelStatusManuallyDisabled).Error)
			case "channel_missing":
				require.NoError(t, model.DB.Delete(&model.Channel{}, rule.ChannelId).Error)
			case "capability_missing":
				require.NoError(t, model.DB.Where("channel_id = ?", rule.ChannelId).Delete(&model.Ability{}).Error)
			case "route_group_invalid":
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			}
			c, recorder := customerContractTemplateContext(http.MethodGet, "/api/customer-contract-templates", "", admin, template.Id)
			GetCustomerContractTemplate(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"unavailable_reason":"`+reason+`"`)
			assert.Contains(t, recorder.Body.String(), `"public_model":"`+rule.PublicModel+`"`)
			c, recorder = customerContractAdminContext(http.MethodGet, "/api/user/contract", "", admin, user)
			GetCustomerContract(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"unavailable_reason":"`+reason+`"`)
			assert.Contains(t, recorder.Body.String(), `"model":"`+rule.PublicModel+`"`)
			recorder = httptest.NewRecorder()
			self, _ := gin.CreateTestContext(recorder)
			self.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/contract", nil)
			self.Set("id", user.Id)
			GetSelfCustomerContract(self)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "unavailable_reason")
			assert.NotContains(t, recorder.Body.String(), reason)
			assert.NotContains(t, recorder.Body.String(), "channel_id")
		})
	}
}
