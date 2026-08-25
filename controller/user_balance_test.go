package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type userBalanceResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Balance  float64 `json:"balance"`
		Currency string  `json:"currency"`
	} `json:"data"`
}

func TestGetUserBalanceReturnsWalletUSDAtFullPrecision(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{
		Username: "balance-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Quota:    617283,
		Group:    "default",
	}
	require.NoError(t, db.Create(&user).Error)
	context, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/user/balance", nil, user.Id)

	GetUserBalance(context)

	var response userBalanceResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 1.234566, response.Data.Balance)
	assert.Equal(t, "USD", response.Data.Currency)
	var responseFields map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responseFields))
	assert.NotContains(t, responseFields, "message")
}

func TestGetUserBalanceClampsNegativeWalletToZero(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{
		Username: "negative-balance-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Quota:    -1,
		Group:    "default",
	}
	require.NoError(t, db.Create(&user).Error)
	context, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/user/balance", nil, user.Id)

	GetUserBalance(context)

	var response userBalanceResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Zero(t, response.Data.Balance)
}
