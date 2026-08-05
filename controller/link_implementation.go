package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ListLinkImplementations(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    model.ListSelectableLinkImplementations(),
	})
}
