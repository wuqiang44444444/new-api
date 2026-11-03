package service

import "github.com/gin-gonic/gin"

// Actual delivery is observed once by the outer writer. Task handlers only
// attach facts; they must not finish an outer response presenter prematurely.
func FinishTaskRequestEvidenceClientDelivery(c *gin.Context) {
	session := evidenceSessionFrom(c)
	if session == nil {
		return
	}
	updateRequestEvidenceIdentity(c, session.capture)
	if session.standalone && !c.GetBool(taskCreateEvidenceDeliveryKey) {
		if writer, ok := c.Writer.(*imageEvidenceWriter); ok {
			writer.prepareDelivery()
		}
		session.capture.Finish(false)
	}
}
