package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// PreviewBillingExpression 是管理员只读表达式试算入口：投影 + 可选真实
// 引擎试算。无任何落账副作用；权限沿用模型价格编辑所在的 option 路由组。
func PreviewBillingExpression(c *gin.Context) {
	var request struct {
		Items []service.BillingExprPreviewItem `json:"items"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if len(request.Items) == 0 || len(request.Items) > service.BillingExprPreviewMaxItems {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("items must contain 1 to %d expressions", service.BillingExprPreviewMaxItems),
		})
		return
	}
	results := service.PreviewBillingExpressions(request.Items)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}
