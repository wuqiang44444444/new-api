package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// registerCustomerContractTemplateRoutes wires the shared contract template
// management API. Static segments are declared before the :id wildcard so
// gin resolves /options independently of /:id.
func registerCustomerContractTemplateRoutes(apiRouter *gin.RouterGroup) {
	route := apiRouter.Group("/customer-contract-templates")
	route.Use(middleware.AdminAuth())
	route.GET("", controller.GetCustomerContractTemplates)
	route.GET("/options", controller.GetCustomerContractTemplateOptions)
	route.POST("", controller.PostCustomerContractTemplate)
	route.GET("/:id", controller.GetCustomerContractTemplate)
	route.PUT("/:id", controller.PutCustomerContractTemplate)
	route.GET("/:id/audits", controller.GetCustomerContractTemplateAudits)
}
