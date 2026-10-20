package controller

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/gin-gonic/gin"
)

func attachModelArkVideoErrorDiagnostics(c *gin.Context, code string) {
	if code != "task_not_found" {
		return
	}
	// An invisible task has no authorized channel or model to report. Do not
	// query outside the caller's scope to distinguish missing from forbidden.
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Stage:      "task_lookup",
		Reason:     "task_not_found",
		PublicCode: "task_not_found",
		Protocol:   "modelark_v3",
	})
}
