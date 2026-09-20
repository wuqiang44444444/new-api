package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customerContractTemplateContext(method string, path string, body string, admin model.User, templateId int) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if templateId > 0 {
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", templateId)}}
	}
	c.Set("id", admin.Id)
	c.Set("role", admin.Role)
	c.Set("username", admin.Username)
	return c, recorder
}

type customerContractTemplateSnapshotResponse struct {
	Success bool                              `json:"success"`
	Data    *customerContractTemplateSnapshot `json:"data"`
}

type customerContractTemplateSnapshot struct {
	Id        int                                    `json:"id"`
	Name      string                                 `json:"name"`
	Enabled   bool                                   `json:"enabled"`
	Version   int64                                  `json:"version"`
	CreatorId int                                    `json:"creator_id"`
	UpdaterId int                                    `json:"updater_id"`
	Rules     []customerContractTemplateSnapshotRule `json:"rules"`
}

type customerContractTemplateSnapshotRule struct {
	Model      string `json:"public_model"`
	ChannelId  int    `json:"channel_id"`
	RouteGroup string `json:"route_group"`
	Discount   string `json:"discount"`
	Available  bool   `json:"available"`
}

func TestCustomerContractTemplateAdminAPILifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, _, contract := setupCustomerContractControllerDB(t)

	c, recorder := customerContractTemplateContext(http.MethodPost, "/api/customer-contract-templates", fmt.Sprintf(`{
		"enabled":true,"name":"Standard Template","reason":"shared baseline",
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"80%%"}]
	}`, contract.Rules[0].ChannelId), admin, 0)

	PostCustomerContractTemplate(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"version":1`)
	assert.Contains(t, recorder.Body.String(), fmt.Sprintf(`"creator_id":%d`, admin.Id))
	var response customerContractTemplateSnapshotResponse
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &response))
	require.True(t, response.Success)
	require.NotNil(t, response.Data)
	templateId := response.Data.Id
	assert.EqualValues(t, 1, response.Data.Version)
	require.Len(t, response.Data.Rules, 1)
	assert.Equal(t, "contract-model", response.Data.Rules[0].Model)
	assert.True(t, response.Data.Rules[0].Available)

	c, recorder = customerContractTemplateContext(http.MethodGet, "/api/customer-contract-templates/options", "", admin, 0)
	GetCustomerContractTemplateOptions(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var options struct {
		Success bool `json:"success"`
		Data    struct {
			Options []struct {
				Group             string `json:"group"`
				NativeGroupRatio  string `json:"native_group_ratio"`
				SpecialGroupRatio bool   `json:"special_group_ratio"`
			} `json:"options"`
			Channels []struct {
				Group             string `json:"group"`
				SpecialGroupRatio bool   `json:"special_group_ratio"`
			} `json:"channels"`
			CustomerContext bool `json:"customer_context"`
		} `json:"data"`
	}
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &options))
	require.True(t, options.Success)
	assert.False(t, options.Data.CustomerContext)
	require.NotEmpty(t, options.Data.Channels, "concrete groups are listed")
	for _, group := range options.Data.Options {
		assert.False(t, group.SpecialGroupRatio, "template options never apply a customer special ratio")
	}
	contractAPIRatio := ""
	for _, group := range options.Data.Options {
		if group.Group == "contract-api" {
			contractAPIRatio = group.NativeGroupRatio
		}
	}
	assert.Equal(t, "0.87", contractAPIRatio, "the reference ratio is the plain native group ratio")

	// Optimistic concurrency: a stale expected_version conflicts with 409.
	staleBody := fmt.Sprintf(`{"expected_version":99,"enabled":true,"name":"Standard Template","reason":"stale edit",
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"70%%"}]}`, contract.Rules[0].ChannelId)
	c, recorder = customerContractTemplateContext(http.MethodPut, fmt.Sprintf("/api/customer-contract-templates/%d", templateId), staleBody, admin, templateId)
	PutCustomerContractTemplate(c)
	assert.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())

	disableBody := fmt.Sprintf(`{"expected_version":1,"enabled":false,"name":"Standard Template","reason":"pause template",
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"80%%"}]}`, contract.Rules[0].ChannelId)
	c, recorder = customerContractTemplateContext(http.MethodPut, fmt.Sprintf("/api/customer-contract-templates/%d", templateId), disableBody, admin, templateId)
	PutCustomerContractTemplate(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"enabled":false`)

	c, recorder = customerContractTemplateContext(http.MethodGet, fmt.Sprintf("/api/customer-contract-templates/%d/audits", templateId), "", admin, templateId)
	GetCustomerContractTemplateAudits(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"operation":"create"`)
	assert.Contains(t, recorder.Body.String(), `"operation":"disable"`)
}

func TestPostCustomerContractEntityAppliesTemplateSourceAndHidesItFromUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin, user, contract := setupCustomerContractControllerDB(t)
	channelId := contract.Rules[0].ChannelId

	created, err := model.CreateCustomerContractTemplate(model.CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Creation Source", Enabled: true, Reason: "api source fixture",
		Rules: []model.CustomerContractEntityRuleInput{
			{PublicModel: "contract-model", ChannelId: channelId, RouteGroup: "contract-api", RatioUnits: 80_000_000},
		},
	})
	require.NoError(t, err)

	c, recorder := customerContractAdminContext(http.MethodPost, fmt.Sprintf("/api/user/%d/contract", user.Id), fmt.Sprintf(`{
		"enabled":true,"name":"Via Template","reason":"apply template",
		"source_template_id":%d,"source_template_version":1,
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"7折"}]
	}`, created.Id, channelId), admin, user)

	PostCustomerContractEntity(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), fmt.Sprintf(`"source_template_id":%d`, created.Id))
	assert.Contains(t, recorder.Body.String(), `"source_template_name":"Creation Source"`)
	assert.Contains(t, recorder.Body.String(), `"discount":"0.7"`)

	// Stale confirmed version: conflict, nothing created.
	beforeStale := countCustomerContracts(t, user.Id)
	c, recorder = customerContractAdminContext(http.MethodPost, fmt.Sprintf("/api/user/%d/contract", user.Id), fmt.Sprintf(`{
		"enabled":true,"name":"Stale Source","reason":"stale version",
		"source_template_id":%d,"source_template_version":99,
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"0.7"}]
	}`, created.Id, channelId), admin, user)
	PostCustomerContractEntity(c)
	assert.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	assert.Equal(t, beforeStale, countCustomerContracts(t, user.Id), "a conflicted template source creates nothing")

	// Half-provided source pair: bad request, nothing created.
	beforeHalf := countCustomerContracts(t, user.Id)
	c, recorder = customerContractAdminContext(http.MethodPost, fmt.Sprintf("/api/user/%d/contract", user.Id), fmt.Sprintf(`{
		"enabled":true,"name":"Half Source","reason":"missing version",
		"source_template_id":%d,
		"rules":[{"model":"contract-model","channel_id":%d,"route_group":"contract-api","discount":"0.7"}]
	}`, created.Id, channelId), admin, user)
	PostCustomerContractEntity(c)
	assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	assert.Equal(t, beforeHalf, countCustomerContracts(t, user.Id), "a half-provided source pair creates nothing")

	// The owner-visible contract response never contains template source.
	c, recorder = customerContractAdminContext(http.MethodGet, fmt.Sprintf("/api/user/%d/contract", user.Id), "", admin, user)
	GetCustomerContract(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "source_template_id")

	recorder = httptest.NewRecorder()
	self, _ := gin.CreateTestContext(recorder)
	self.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/contract", nil)
	self.Set("id", user.Id)
	GetSelfCustomerContract(self)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "source_template")
	assert.NotContains(t, recorder.Body.String(), "template_id")
}

func countCustomerContracts(t *testing.T, userId int) int {
	t.Helper()
	contracts, err := model.ListContractEntitiesForUser(userId, false)
	require.NoError(t, err)
	return len(contracts)
}
