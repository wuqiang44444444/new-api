package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractTokenEditPreservesUnchangedRevokedAutoGroups(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	token := model.Token{UserId: user.Id, Key: "contract-edit-test", Name: "original", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto", CrossGroupRetry: true, ContractId: contract.Id}
	require.NoError(t, token.SetAutoGroups([]string{"revoked", "default"}))
	require.NoError(t, model.DB.Create(&token).Error)
	for _, tc := range []struct {
		groups  []string
		success bool
	}{{[]string{"revoked", "default"}, true}, {[]string{"default", "revoked"}, false}} {
		request := map[string]any{"id": token.Id, "name": "renamed", "group": "auto", "auto_groups": tc.groups, "cross_group_retry": true, "contract_id": contract.Id, "unlimited_quota": true, "expired_time": -1}
		ctx, recorder := newTokenAutoGroupsAuthenticatedContext(t, http.MethodPut, "/api/token/", request, user.Id)
		UpdateToken(ctx)
		response := decodeAPIResponse(t, recorder)
		assert.Equal(t, tc.success, response.Success, response.Message)
		var stored model.Token
		require.NoError(t, model.DB.First(&stored, token.Id).Error)
		assert.Equal(t, "renamed", stored.Name)
		assert.Equal(t, token.AutoGroups, stored.AutoGroups)
		assert.True(t, stored.CrossGroupRetry)
	}
}
