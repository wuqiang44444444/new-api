package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func GetTaskRequestEvidenceRetention(c *gin.Context) {
	common.ApiSuccess(c, system_setting.GetTaskRequestEvidenceRetentionPolicy())
}
