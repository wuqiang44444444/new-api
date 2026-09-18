package controller

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetSelfBillingStatementLogs(c *gin.Context)  { getBillingStatementLogs(c, false) }
func GetAdminBillingStatementLogs(c *gin.Context) { getBillingStatementLogs(c, true) }

func getBillingStatementLogs(c *gin.Context, admin bool) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	userId, role := c.GetInt("id"), common.RoleCommonUser
	if admin {
		var err error
		userId, err = strconv.Atoi(c.Query("user_id"))
		if err != nil || userId <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user_id"})
			return
		}
		role = c.GetInt("role")
	}
	modelName, mode, ok := parseBillingReconciliationModelFilters(c)
	if !ok {
		return
	}
	filter := model.BillingStatementLogFilter{UserId: userId, Start: period.StartTimestamp, End: period.EndTimestamp, ModelName: modelName, BillingMode: mode,
		TokenName: c.Query("token_name"), Group: c.Query("group"), RequestId: c.Query("request_id"), UpstreamRequestId: c.Query("upstream_request_id")}
	if admin {
		filter.Username = c.Query("username")
	}
	for name, target := range map[string]**int{"token_id": &filter.TokenId, "channel": &filter.ChannelId} {
		if raw, present := c.GetQuery(name); present {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid " + name})
				return
			}
			*target = &value
		}
	}
	if raw, present := c.GetQuery("type"); present {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > model.LogTypeLogin {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid type"})
			return
		}
		filter.LogType = value
	}
	page := common.GetPageQuery(c)
	if page.GetPage() < 1 || page.GetPageSize() < 1 || int64(page.GetPage()-1) > math.MaxInt64/int64(page.GetPageSize()) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid pagination"})
		return
	}
	result, err := model.GetBillingStatementLogs(c.Request.Context(), filter, page.GetPage(), page.GetPageSize(), role)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.HasSuffix(c.FullPath(), "/stat") {
		common.ApiSuccess(c, gin.H{"quota": result.Quota, "rpm": result.RPM, "tpm": result.TPM})
		return
	}
	service.AttachLogsBillingDisplay(result.Items)
	common.ApiSuccess(c, result)
}
