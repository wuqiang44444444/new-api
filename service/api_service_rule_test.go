package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIServiceRuleAcceptanceIsBoundToAppAndExactContent(t *testing.T) {
	truncate(t)
	rule, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{
		Version: "2026-08-02", Title: "API service rules", Content: "application compliance responsibilities",
	})
	require.NoError(t, err)
	require.NoError(t, model.ActivateAPIServiceRule(rule.ID))

	_, err = AcceptCurrentAPIServiceRule(7, 101, dto.AcceptAPIServiceRuleRequest{
		Version: rule.Version, ContentSHA256: "wrong",
		ComplianceOwner: "App Compliance", ConsentRecordSystemRef: "consent-ledger/app-101",
	})
	require.ErrorIs(t, err, ErrAPIServiceRuleMismatch)
	require.ErrorIs(t, RequireCurrentAPIServiceRuleAcceptance(7, 101), ErrAPIServiceRuleNotAccepted)

	acceptance, err := AcceptCurrentAPIServiceRule(7, 101, dto.AcceptAPIServiceRuleRequest{
		Version: rule.Version, ContentSHA256: rule.ContentSHA256,
		ComplianceOwner: "App Compliance", ConsentRecordSystemRef: "consent-ledger/app-101",
	})
	require.NoError(t, err)
	assert.Equal(t, 101, acceptance.AppID)
	assert.Equal(t, "token:101", acceptance.AcceptedBy)
	assert.Equal(t, "api_token", acceptance.AcceptanceMethod)
	require.NoError(t, RequireCurrentAPIServiceRuleAcceptance(7, 101))
	require.ErrorIs(t, RequireCurrentAPIServiceRuleAcceptance(7, 102), ErrAPIServiceRuleNotAccepted)
}

func TestActivatingNewAPIServiceRuleRequiresReacceptance(t *testing.T) {
	truncate(t)
	first, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{Version: "v1", Title: "Rules", Content: "first"})
	require.NoError(t, err)
	require.NoError(t, model.ActivateAPIServiceRule(first.ID))
	_, err = AcceptCurrentAPIServiceRule(8, 201, dto.AcceptAPIServiceRuleRequest{
		Version: first.Version, ContentSHA256: first.ContentSHA256,
		ComplianceOwner: "owner", ConsentRecordSystemRef: "ledger/201",
	})
	require.NoError(t, err)

	second, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{Version: "v2", Title: "Rules", Content: "second"})
	require.NoError(t, err)
	require.NoError(t, model.ActivateAPIServiceRule(second.ID))
	require.ErrorIs(t, RequireCurrentAPIServiceRuleAcceptance(8, 201), ErrAPIServiceRuleNotAccepted)

	rules, err := model.ListAPIServiceRules()
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, model.APIServiceRuleActive, rules[0].Status)
	assert.Equal(t, model.APIServiceRuleRetired, rules[1].Status)
}

func TestAPIServiceRuleAcceptanceIsImmutable(t *testing.T) {
	truncate(t)
	rule, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{Version: "immutable-v1", Title: "Rules", Content: "terms"})
	require.NoError(t, err)
	require.NoError(t, model.ActivateAPIServiceRule(rule.ID))

	first, err := AcceptCurrentAPIServiceRule(9, 301, dto.AcceptAPIServiceRuleRequest{
		Version: rule.Version, ContentSHA256: rule.ContentSHA256,
		ComplianceOwner: "original owner", ConsentRecordSystemRef: "ledger/original",
	})
	require.NoError(t, err)
	second, err := AcceptCurrentAPIServiceRule(9, 301, dto.AcceptAPIServiceRuleRequest{
		Version: rule.Version, ContentSHA256: rule.ContentSHA256,
		ComplianceOwner: "replacement owner", ConsentRecordSystemRef: "ledger/replacement",
	})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, first.AcceptedAt, second.AcceptedAt)
	assert.Equal(t, "original owner", second.ComplianceOwner)
	assert.Equal(t, "ledger/original", second.ConsentRecordSystemRef)
	err = model.DB.Model(&model.ApplicationAPIRuleAcceptance{}).
		Where("id = ?", first.ID).
		Update("compliance_owner", "direct overwrite").Error
	require.ErrorIs(t, err, model.ErrAPIServiceRuleAcceptanceImmutable)
}

func TestFutureAPIServiceRuleCannotRetireCurrentRule(t *testing.T) {
	truncate(t)
	current, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{Version: "current", Title: "Rules", Content: "current"})
	require.NoError(t, err)
	require.NoError(t, model.ActivateAPIServiceRule(current.ID))
	future, err := CreateAPIServiceRule(dto.CreateAPIServiceRuleRequest{
		Version: "future", Title: "Rules", Content: "future", EffectiveAt: common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)

	require.ErrorIs(t, model.ActivateAPIServiceRule(future.ID), model.ErrAPIServiceRuleNotEffective)
	active, err := model.GetActiveAPIServiceRule()
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, current.ID, active.ID)
}
