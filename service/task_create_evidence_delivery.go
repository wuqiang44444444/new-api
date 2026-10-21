package service

import "github.com/gin-gonic/gin"

const taskCreateEvidenceDeliveryKey = "task_create_evidence_delivery"

// BeginTaskCreateEvidenceDelivery observes the final writes outside the
// response-contract buffer. Inner task handlers must not finish this capture
// before the presenter replaces their internal response with the public one.
func BeginTaskCreateEvidenceDelivery(c *gin.Context) func() {
	if !evidenceEnabled() {
		return func() {}
	}
	original := c.Writer
	writer := &evidenceClientWriter{ResponseWriter: original}
	c.Writer = writer
	c.Set(taskCreateEvidenceDeliveryKey, true)
	return func() {
		defer func() { c.Writer = original }()
		c.Set(taskCreateEvidenceDeliveryKey, false)
		c.Writer = writer
		FinishTaskRequestEvidenceClientDelivery(c)
	}
}
