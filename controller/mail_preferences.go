package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type mailPreferenceView struct {
	model.MailTypeDefinition
	Enabled bool `json:"enabled"`
}

func mailPreferencesView(c *gin.Context, prefs model.MailPreferences) gin.H {
	catalog := model.MailTypeCatalog()
	view := make([]mailPreferenceView, 0, len(catalog))
	for _, entry := range catalog {
		view = append(view, mailPreferenceView{entry, prefs.Enabled(entry.ID)})
	}
	finance, _ := model.FinanceMailRecipients(model.DB.WithContext(c.Request.Context()))
	return gin.H{"types": view, "finance_recipients": strings.Join(finance, ", ")}
}

func getMailPreferences(c *gin.Context) {
	prefs, err := model.LoadMailPreferences(c.Request.Context())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, mailPreferencesView(c, prefs))
}

func updateMailPreferences(c *gin.Context) {
	var requested struct {
		Types             map[model.MailType]*bool `json:"types"`
		FinanceRecipients *string                  `json:"finance_recipients"`
	}
	if err := common.DecodeJson(c.Request.Body, &requested); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	changes := make(map[model.MailType]bool, len(requested.Types))
	for kind, enabled := range requested.Types {
		if enabled == nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		changes[kind] = *enabled
	}
	prefs, err := model.UpdateMailPreferences(c.Request.Context(), changes, requested.FinanceRecipients)
	if errors.Is(err, model.ErrInvalidMailPreferences) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	recordManageAuditFor(c, c.GetInt("id"), "option.update", map[string]any{"key": "mail_preferences", "changes": changes, "finance_recipients_changed": requested.FinanceRecipients != nil})
	common.ApiSuccess(c, mailPreferencesView(c, prefs))
}
