package service

import "github.com/gin-gonic/gin"

const taskCreateEvidenceDeliveryKey = "task_create_evidence_delivery"

func BeginTaskCreateEvidenceDelivery(c *gin.Context) func() {
	// Install the canonical observer before the response presenter buffers its
	// internal response. Standalone routers must also record the public bytes.
	if c.Request != nil && apiEvidenceFrom(c.Request.Context()) == nil {
		_ = BeginTaskRequestEvidence(c, mediaEvidenceContextKind(c))
	}
	c.Set(taskCreateEvidenceDeliveryKey, true)
	return func() {
		c.Set(taskCreateEvidenceDeliveryKey, false)
		FinishTaskRequestEvidenceClientDelivery(c)
	}
}
