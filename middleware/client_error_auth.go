package middleware

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/gin-gonic/gin"
)

// Shared by required dashboard authentication and the successfully matched
// credential branch of optional authentication. Anonymous branches never call it.
func markDashboardClientErrorAuth(c *gin.Context, useAccessToken bool) {
	source := clienterrlog.AuthSourceSession
	if useAccessToken {
		source = clienterrlog.AuthSourcePAT
	}
	clienterrlog.MarkAuthPassed(c, source)
}
