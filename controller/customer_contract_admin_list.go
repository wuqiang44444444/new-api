package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetCustomerContracts(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if !model.IsCustomerContractAdminStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid customer contract status"})
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if len(keyword) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "customer contract search keyword is too long"})
		return
	}
	page := common.GetPageQuery(c)
	if page.PageSize < 1 {
		page.PageSize = common.ItemsPerPage
	}
	items, total, summary, err := model.GetCustomerContractAdminList(model.CustomerContractAdminListFilter{
		AdminRole: c.GetInt("role"),
		Keyword:   keyword,
		Status:    status,
		Offset:    page.GetStartIdx(),
		Limit:     page.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"page": page.GetPage(), "page_size": page.GetPageSize(), "total": total,
		"items": items, "summary": summary,
	})
}
