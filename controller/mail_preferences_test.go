package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailPreferencesEndpointSavesOnlyRequestedTypes(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	router := gin.New()
	RegisterMailSettingsRoutes(router.Group("/option"))
	type response struct {
		Success bool `json:"success"`
		Data    struct {
			Types   []mailPreferenceView `json:"types"`
			Finance string               `json:"finance_recipients"`
		} `json:"data"`
	}
	call := func(method, body string) response {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, "/option/mail/preferences", strings.NewReader(body)))
		var result response
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}
	initial := call(http.MethodGet, "")
	require.True(t, initial.Success)
	assert.Contains(t, initial.Data.Types, mailPreferenceView{
		MailTypeDefinition: model.MailTypeDefinition{ID: model.MailWalletRejection, Group: "alerts", Label: "Requests rejected due to insufficient wallet balance", Recipient: "operations"},
		Enabled:            false,
	})
	updated := call(http.MethodPut, `{"types":{"channel_upstream_balance":false},"finance_recipients":"finance@example.com"}`)
	require.True(t, updated.Success)
	assert.Equal(t, "finance@example.com", updated.Data.Finance)
	for i, old := range initial.Data.Types {
		if old.ID == model.MailChannelBalance {
			old.Enabled = false
		}
		assert.Equal(t, old, updated.Data.Types[i])
	}
	assert.False(t, call(http.MethodPut, `{"types":{"unknown":false},"finance_recipients":"changed@example.com"}`).Success)
	assert.False(t, call(http.MethodPut, `{"types":{"channel_upstream_balance":null}}`).Success)
	assert.Equal(t, updated, call(http.MethodGet, ""))
}
