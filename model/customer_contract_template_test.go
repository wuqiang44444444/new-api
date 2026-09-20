package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractTemplateLifecycleAndRuleSemantics(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, _ := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "template-model", common.ChannelStatusEnabled)
	otherChannel := createCustomerContractAbility(t, db, "contract-b", "template-model", common.ChannelStatusEnabled)

	template, err := CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Standard", Enabled: true,
		Reason: "create standard template",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "template-model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 80_000_000},
		},
	})
	require.NoError(t, err)
	assert.True(t, template.Enabled)
	assert.EqualValues(t, 1, template.Version)
	assert.Equal(t, admin.Id, template.CreatorId)
	assert.Equal(t, admin.Id, template.UpdaterId)
	require.Len(t, template.Rules, 2)
	assert.True(t, template.Rules[0].Available)

	// Same model on another channel must keep one identical discount.
	_, err = CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Conflicting discount", Enabled: true,
		Reason: "reject conflicting discounts",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "template-model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 50_000_000},
		},
	})
	require.ErrorIs(t, err, ErrCustomerContractInvalidRule)

	_, err = CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Case duplicate", Enabled: true,
		Reason: "reject case-only duplicates",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "Template-Model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 80_000_000},
		},
	})
	require.ErrorIs(t, err, ErrCustomerContractInvalidRule)

	_, err = CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Empty", Enabled: true, Reason: "reject empty rules",
	})
	require.ErrorIs(t, err, ErrCustomerContractInvalidRule)

	// A stale stored source does not block editing other rules or disabling.
	require.NoError(t, db.Delete(&Channel{}, channel.Id).Error)
	disabled := false
	updated, err := ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
		TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: template.Version,
		Name: "Standard", Enabled: &disabled, Reason: "disable with stale rule",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "template-model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 80_000_000},
		},
	})
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	assert.EqualValues(t, 2, updated.Version)
	require.Len(t, updated.Rules, 2, "stale rules are kept, never filtered")
	staleCount := 0
	for _, rule := range updated.Rules {
		if !rule.Available {
			staleCount++
		}
	}
	assert.Equal(t, 1, staleCount, "exactly the deleted-channel rule is stale")

	enabled := true
	restored, err := ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
		TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: updated.Version,
		Name: "Standard", Enabled: &enabled, Reason: "restore template",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 80_000_000},
		},
	})
	require.NoError(t, err)
	assert.True(t, restored.Enabled)
	assert.EqualValues(t, 3, restored.Version)
	require.Len(t, restored.Rules, 1)
	assert.True(t, restored.Rules[0].Available)

	// Optimistic concurrency: stale versions lose and change nothing.
	_, err = ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
		TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: 1,
		Name: "Standard", Reason: "stale editor must lose",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "template-model", ChannelId: otherChannel.Id, RouteGroup: "contract-b", RatioUnits: 50_000_000},
		},
	})
	require.ErrorIs(t, err, ErrCustomerContractTemplateVersionConflict)
	current, err := GetContractTemplateSnapshot(template.Id, false)
	require.NoError(t, err)
	assert.EqualValues(t, 3, current.Version)
	assert.EqualValues(t, 80_000_000, current.Rules[0].RatioUnits)

	audits, total, err := GetContractTemplateAudits(template.Id, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, audits, 3)
	assert.Equal(t, "create", audits[2].Operation)
	assert.Equal(t, "disable", audits[1].Operation)
	assert.Equal(t, "enable", audits[0].Operation)
	assert.Equal(t, admin.Id, audits[0].AdminUserId)
}

func TestCustomerContractTemplateListCountsAndFilters(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, _ := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "list-model", common.ChannelStatusEnabled)
	deadChannel := Channel{Name: "contract-a-dead", Group: "contract-a", Models: "list-model,stale-list-model", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&deadChannel).Error)
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "list-model", ChannelId: deadChannel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "stale-list-model", ChannelId: deadChannel.Id, Enabled: true}).Error)

	_, err := CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Alpha template", Enabled: true, Reason: "list fixture",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "list-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 90_000_000},
			{PublicModel: "list-model", ChannelId: deadChannel.Id, RouteGroup: "contract-a", RatioUnits: 90_000_000},
			{PublicModel: "stale-list-model", ChannelId: deadChannel.Id, RouteGroup: "contract-a", RatioUnits: 90_000_000},
		},
	})
	require.NoError(t, err)
	// Sources that die after acceptance stay in the template and are only
	// derived as stale, never filtered out.
	require.NoError(t, db.Delete(&Channel{}, deadChannel.Id).Error)
	_, err = CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Beta template", Enabled: false, Reason: "list fixture",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "list-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 90_000_000},
		},
	})
	require.NoError(t, err)

	items, total, err := GetCustomerContractTemplateList("", nil, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	// Newest template first.
	assert.Equal(t, "Beta template", items[0].Name)
	assert.False(t, items[0].Enabled)
	assert.Equal(t, admin.Id, items[0].UpdaterId)
	assert.Equal(t, "contract-admin", items[0].UpdaterName)

	alpha := items[1]
	assert.Equal(t, "Alpha template", alpha.Name)
	assert.Equal(t, 2, alpha.ModelCount, "model count dedupes public models")
	assert.Equal(t, 3, alpha.RuleCount, "rule count counts channel rules")
	assert.Equal(t, 2, alpha.StaleCount, "stale rules are derived, not persisted")

	enabledFilter := true
	filtered, total, err := GetCustomerContractTemplateList("", &enabledFilter, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Alpha template", filtered[0].Name)

	byName, total, err := GetCustomerContractTemplateList("alpha", nil, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, byName, 1)
	assert.Equal(t, "Alpha template", byName[0].Name)
}

func TestCustomerContractTemplateSourcedCreation(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "source-model", common.ChannelStatusEnabled)

	template, err := CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
		AdminUserId: admin.Id, Name: "Source", Enabled: true, Reason: "source fixture",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "source-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
		},
	})
	require.NoError(t, err)

	rules := []CustomerContractEntityRuleInput{
		{PublicModel: "source-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 70_000_000},
	}
	contract, err := CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "From template", Enabled: true,
		Reason: "created from template", Rules: rules,
		SourceTemplateId: template.Id, SourceTemplateVersion: template.Version,
	})
	require.NoError(t, err)
	assert.True(t, contract.Enabled)
	assert.Equal(t, template.Id, contract.SourceTemplateId)
	assert.EqualValues(t, template.Version, contract.SourceTemplateVersion)
	assert.Equal(t, "Source", contract.SourceTemplateName)
	require.Len(t, contract.Rules, 1)
	assert.EqualValues(t, 70_000_000, contract.Rules[0].RatioUnits,
		"the final rules are the submitted ones, not a template copy")

	// The plain creation path stays untouched and carries no source.
	plain, err := CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Plain", Enabled: true,
		Reason: "plain creation", Rules: rules,
	})
	require.NoError(t, err)
	assert.Zero(t, plain.SourceTemplateId)
	assert.Empty(t, plain.SourceTemplateName)

	// Template modified after apply: the confirmed version no longer matches.
	_, err = ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
		TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: template.Version,
		Name: "Source", Reason: "bump after apply",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "source-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 60_000_000},
		},
	})
	require.NoError(t, err)
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Stale version", Enabled: true,
		Reason: "stale template version", Rules: rules,
		SourceTemplateId: template.Id, SourceTemplateVersion: template.Version,
	})
	require.ErrorIs(t, err, ErrCustomerContractTemplateVersionConflict)

	// Final rule validation still runs for the template branch: an invalid
	// source in the submitted rules is rejected even with a valid template.
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Invalid rules", Enabled: true,
		Reason: "invalid final rules", Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "source-model", ChannelId: channel.Id + 50_000, RouteGroup: "contract-a", RatioUnits: 70_000_000},
		},
		SourceTemplateId: template.Id, SourceTemplateVersion: template.Version + 1,
	})
	require.Error(t, err)

	// Disabled templates cannot create contracts.
	disabled := false
	_, err = ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
		TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: template.Version + 1,
		Name: "Source", Enabled: &disabled, Reason: "disable template",
		Rules: []CustomerContractEntityRuleInput{
			{PublicModel: "source-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 60_000_000},
		},
	})
	require.NoError(t, err)
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Disabled source", Enabled: true,
		Reason: "disabled template source", Rules: rules,
		SourceTemplateId: template.Id, SourceTemplateVersion: template.Version + 1,
	})
	require.ErrorIs(t, err, ErrCustomerContractTemplateDisabled)

	// Missing templates cannot create contracts.
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Missing source", Enabled: true,
		Reason: "missing template source", Rules: rules,
		SourceTemplateId: template.Id + 999, SourceTemplateVersion: 1,
	})
	require.ErrorIs(t, err, ErrCustomerContractTemplateNotFound)

	// Paired fields are mandatory together.
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Name: "Half source", Enabled: true,
		Reason: "half template source", Rules: rules,
		SourceTemplateVersion: 1,
	})
	require.Error(t, err)

	// After all of this the failed creations left no contract rows: the
	// transaction contains the template check, rules and audit together.
	contracts, err := ListContractEntitiesForUser(user.Id, false)
	require.NoError(t, err)
	require.Len(t, contracts, 2)
	for _, listed := range contracts {
		assert.Contains(t, []string{"From template", "Plain"}, listed.Name)
	}
}
