package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func SendPasswordResetEmail(c *gin.Context) {
	setAuthNoStore(c)
	email := model.NormalizeEmail(c.Query("email"))
	if err := common.Validate.Var(email, "required,email,max=50"); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	language := i18n.StatedLang(c)
	if !service.EnqueueMailWork(func() { service.DeliverPasswordReset(email, language) }) {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Mail delivery is busy"), nil)
		return
	}
	// This response is the same for unknown, disabled and valid accounts.
	common.ApiSuccess(c, nil)
}

func ResetPassword(c *gin.Context) {
	setAuthNoStore(c)
	var request struct {
		Token           string `json:"token"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || len(request.Token) != 43 {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}
	if request.Password != request.ConfirmPassword {
		common.ApiErrorT(c, "Passwords do not match")
		return
	}
	if err := common.ValidateNewAccountPassword(request.Password); err != nil {
		common.ApiErrorT(c, err.Error())
		return
	}
	userID, err := model.CompletePasswordReset(request.Token, request.Password)
	switch {
	case errors.Is(err, model.ErrAuthFlowInvalid), errors.Is(err, model.ErrAuthFlowExpired), errors.Is(err, model.ErrAuthFlowConsumed):
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
	case errors.Is(err, common.ErrPasswordLegacyLimit):
		common.ApiErrorT(c, err.Error())
	case err != nil:
		common.SysError("password reset transaction failed")
		common.ApiErrorT(c, "Could not reset password")
	default:
		if err := model.PublishUserAuthCache(userID); err != nil {
			common.SysError("password reset auth cache publication failed")
		}
		if _, err := model.RevokeAllUserSessions(userID, "password_reset"); err != nil {
			common.SysError("password reset session cleanup failed")
		}
		recordUserSecurityAudit(c, userID, "user.password_reset", map[string]any{"success": true})
		common.ApiSuccess(c, nil)
	}
}
