package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func RegisterMailSettingsRoutes(options *gin.RouterGroup) {
	routes := options.Group("/mail", middleware.DisableCache())
	routes.GET("", getMailTransport)
	routes.PUT("", updateMailTransport)
	routes.POST("/test", middleware.UserCriticalRateLimit("mail-test"), sendMailTest)
}

func getMailTransport(c *gin.Context) {
	config, err := model.LoadMailTransport(c.Request.Context())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	hasCredential := false
	if config.SMTP != nil {
		hasCredential = config.SMTP.Token != ""
		config.SMTP.Token = ""
	}
	if config.Graph != nil {
		hasCredential = config.Graph.ClientSecret != ""
		config.Graph.ClientSecret = ""
	}
	common.ApiSuccess(c, gin.H{"config": config, "has_credential": hasCredential})
}

func updateMailTransport(c *gin.Context) {
	var config common.MailTransportConfig
	if err := common.DecodeJson(c.Request.Body, &config); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if config.SMTP != nil {
		config.SMTP.Server = strings.TrimSpace(config.SMTP.Server)
		config.SMTP.Account = strings.TrimSpace(config.SMTP.Account)
		config.SMTP.From = strings.TrimSpace(config.SMTP.From)
	}
	if config.Graph != nil {
		config.Graph.TenantID = strings.TrimSpace(config.Graph.TenantID)
		config.Graph.ClientID = strings.TrimSpace(config.Graph.ClientID)
		config.Graph.Sender = strings.TrimSpace(config.Graph.Sender)
		config.Graph.MailboxID = strings.TrimSpace(config.Graph.MailboxID)
	}
	if err := model.UpdateMailTransport(c.Request.Context(), config); err != nil {
		var message *common.Message
		if errors.As(err, &message) {
			common.ApiError(c, message)
		} else {
			common.SysError("mail transport settings update failed")
			common.ApiErrorT(c, "Could not save mail settings")
		}
		return
	}
	recordManageAuditFor(c, c.GetInt("id"), "option.update", map[string]any{"key": "mail_transport", "mode": config.Mode})

	getMailTransport(c)
}

func sendMailTest(c *gin.Context) {
	var request struct {
		Recipient string `json:"recipient"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	recipient := strings.TrimSpace(request.Recipient)
	if !common.IsBareMailAddress(recipient) {
		common.ApiErrorT(c, "Please enter a valid email address")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), common.ShortMailSubmissionTimeout)
	defer cancel()
	if err := common.SendEmailContext(ctx, "邮件通道测试 / Mail delivery test", recipient,
		"<h2>邮件通道测试 / Mail delivery test</h2><p>此邮件由管理员手动触发。收到此信即表示测试邮件已到达。</p><p>An administrator requested this test. Receipt of this message confirms delivery to this mailbox.</p>"); err != nil {
		common.SysLog(common.LogText("manual mail test failed: %s", err.Error()))
		common.ApiErrorT(c, "Test email could not be submitted. Check mail settings and try again.")
		return
	}
	common.ApiSuccess(c, nil)
}
