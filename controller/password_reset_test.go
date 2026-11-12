package controller

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateUserRequiresEmail(t *testing.T) {
	_, identity, _ := setupAdminUserTest(t)
	for _, email := range []string{"", "   ", "not-an-address"} {
		body, err := common.Marshal(map[string]any{
			"username": "no-email", "password": "member-password-1", "role": 1, "email": email,
		})
		require.NoError(t, err)
		response := adminUserRequest(http.MethodPost, "/api/user/", string(body), "", identity, common.RoleAdminUser, nil, CreateUser)
		var result securityEnrollmentResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "no-email").Count(&count).Error)
	assert.Zero(t, count)
}

func TestPasswordResetRequiresConfirmationAndConsumesLink(t *testing.T) {
	user, identity, _ := setupAdminUserTest(t)
	user.Email = "reset@example.com"
	require.NoError(t, model.DB.Model(user).Update("email", user.Email).Error)
	token, err := model.CreatePasswordReset(user)
	require.NoError(t, err)
	_, err = model.CreatePasswordReset(user)
	require.ErrorIs(t, err, model.ErrPasswordResetRecent)
	require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("user_id = ? AND purpose = ?", user.Id, model.AuthFlowPurposePasswordReset).Update("created_at", time.Now().Add(-2*time.Minute)).Error)
	secondToken, err := model.CreatePasswordReset(user)
	require.NoError(t, err)

	reset := func(token, confirm string) (bool, string) {
		body, err := common.Marshal(map[string]string{"token": token, "password": "new-password-1", "confirm_password": confirm})
		require.NoError(t, err)
		response := adminUserRequest(http.MethodPost, "/api/user/reset", string(body), "", identity, 0, nil, ResetPassword)
		var result securityEnrollmentResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		return result.Success, response.Body.String()
	}

	ok, _ := reset(token, "different-password")
	assert.False(t, ok)
	ok, output := reset(token, "new-password-1")
	require.True(t, ok, output)
	assert.False(t, strings.Contains(output, "new-password-1"))
	ok, _ = reset(token, "new-password-1")
	assert.False(t, ok)
	ok, _ = reset(secondToken, "new-password-1")
	assert.False(t, ok)
	stored, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.True(t, common.ValidatePasswordAndHash("new-password-1", stored.Password))
	assert.Greater(t, stored.AuthVersion, user.AuthVersion)
}

func TestDiscardUnsentPasswordResetAllowsImmediateNewLink(t *testing.T) {
	user, _, _ := setupAdminUserTest(t)
	user.Email = "reset@example.com"
	require.NoError(t, model.DB.Model(user).Update("email", user.Email).Error)
	first, err := model.CreatePasswordReset(user)
	require.NoError(t, err)
	require.NoError(t, model.DiscardUnsentPasswordReset(first))
	second, err := model.CreatePasswordReset(user)
	require.NoError(t, err)
	_, err = model.CompletePasswordReset(first, "new-password-1")
	require.ErrorIs(t, err, model.ErrAuthFlowInvalid)
	_, err = model.CompletePasswordReset(second, "new-password-1")
	require.NoError(t, err)
}

func TestPasswordResetRejectsExpiredAndChangedAccount(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		invalidate func(*testing.T, *model.User)
	}{
		{"expired", func(t *testing.T, user *model.User) {
			require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("user_id = ? AND purpose = ?", user.Id, model.AuthFlowPurposePasswordReset).Update("expires_at", time.Now().Add(-time.Minute)).Error)
		}},
		{"email changed", func(t *testing.T, user *model.User) {
			require.NoError(t, model.DB.Model(user).Update("email", "other@example.com").Error)
		}},
		{"account disabled", func(t *testing.T, user *model.User) {
			require.NoError(t, model.DB.Model(user).Update("status", common.UserStatusDisabled).Error)
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			user, _, _ := setupAdminUserTest(t)
			user.Email = "reset@example.com"
			require.NoError(t, model.DB.Model(user).Update("email", user.Email).Error)
			link, err := model.CreatePasswordReset(user)
			require.NoError(t, err)
			scenario.invalidate(t, user)
			_, err = model.CompletePasswordReset(link, "new-password-1")
			require.Error(t, err)
			current, err := model.GetUserById(user.Id, true)
			require.NoError(t, err)
			assert.Equal(t, user.AuthVersion, current.AuthVersion)
			assert.False(t, common.ValidatePasswordAndHash("new-password-1", current.Password))
		})
	}
}

func TestPasswordResetLinkHasOneConcurrentWinner(t *testing.T) {
	user, _, _ := setupAdminUserTest(t)
	user.Email = "reset@example.com"
	require.NoError(t, model.DB.Model(user).Update("email", user.Email).Error)
	link, err := model.CreatePasswordReset(user)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := model.CompletePasswordReset(link, "new-password-1")
			results <- err
		}()
	}
	close(start)
	winners := 0
	for range 2 {
		if <-results == nil {
			winners++
		}
	}
	assert.Equal(t, 1, winners)
	stored, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, user.AuthVersion+1, stored.AuthVersion)
	assert.True(t, common.ValidatePasswordAndHash("new-password-1", stored.Password))
}
