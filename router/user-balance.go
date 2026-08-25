package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerUserBalanceRoutes(apiRouter *gin.RouterGroup) {
	balanceRoute := apiRouter.Group("/user")
	balanceRoute.Use(middleware.TokenAuthReadOnly())
	balanceRoute.GET("/balance", controller.GetUserBalance)
}
