package controller

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

// GetClientErrorLogHealth reads only local atomics; it never uses the log writer.
func GetClientErrorLogHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": struct {
		clienterrlog.Health
		ImageTask     clienterrlog.Health `json:"image_task"`
		ImageEvidence map[string]any      `json:"image_evidence"`
	}{clienterrlog.CurrentHealth(), service.ImageTaskDiagHealth(), service.ImageErrorEvidenceHealth()}})
}
