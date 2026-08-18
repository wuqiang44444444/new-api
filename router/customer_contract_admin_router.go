package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerCustomerContractAdminRoutes(apiRouter *gin.RouterGroup) {
	route := apiRouter.Group("/customer-contracts")
	route.Use(middleware.AdminAuth())
	route.GET("", controller.GetCustomerContracts)
}
