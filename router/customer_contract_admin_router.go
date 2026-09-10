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

	contractRoute := apiRouter.Group("/contract")
	contractRoute.Use(middleware.AdminAuth())
	contractRoute.PUT("/:contract_id", controller.PutCustomerContractEntity)
	contractRoute.GET("/:contract_id/audits", controller.GetCustomerContractEntityAudits)

	migrationRoute := apiRouter.Group("/customer-contracts/migration")
	migrationRoute.Use(middleware.AdminAuth())
	migrationRoute.GET("/preview", controller.GetCustomerContractMigrationPreview)
	migrationRoute.POST("", controller.PostCustomerContractMigration)
}
