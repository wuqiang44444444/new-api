package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidBillingURLGroupKeyAcceptsOnlySummaryIdentities(t *testing.T) {
	assert.True(t, validBillingURLGroupKey("https://api.example.com"))
	assert.True(t, validBillingURLGroupKey("https://api.example.com/v1"))
	assert.True(t, validBillingURLGroupKey("http://api.example.com:8443/base"))
	assert.True(t, validBillingURLGroupKey("channel:24"))
	assert.False(t, validBillingURLGroupKey(""))
	assert.False(t, validBillingURLGroupKey("https://API.EXAMPLE.COM:443/"), "un-normalized spellings are not stored identities")
	assert.False(t, validBillingURLGroupKey("https://user:pass@api.example.com"))
	assert.False(t, validBillingURLGroupKey("https://api.example.com/?key=1"))
	assert.False(t, validBillingURLGroupKey("channel:0"))
	assert.False(t, validBillingURLGroupKey("channel:abc"))
	assert.False(t, validBillingURLGroupKey("junk"))
	assert.False(t, validBillingURLGroupKey(strings.Repeat("x", 2049)))
}

func TestSaveProviderURLGroupNameSaveUpdateClearWithAudit(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	urlKey := "https://api.example.com"

	require.NoError(t, SaveProviderURLGroupName(urlKey, "Primary upstream", 9))
	names, err := getProviderURLGroupNames()
	require.NoError(t, err)
	assert.Equal(t, "Primary upstream", names[urlKey])

	require.NoError(t, SaveProviderURLGroupName(urlKey, "  Renamed  ", 10))
	names, err = getProviderURLGroupNames()
	require.NoError(t, err)
	assert.Equal(t, "Renamed", names[urlKey], "names are plain text, trimmed before storage")

	// A byte-identical resave is a no-op and writes no second audit row.
	require.NoError(t, SaveProviderURLGroupName(urlKey, "Renamed", 10))

	require.NoError(t, SaveProviderURLGroupName(urlKey, "", 11))
	names, err = getProviderURLGroupNames()
	require.NoError(t, err)
	assert.Empty(t, names[urlKey], "clearing restores the default safe-URL label")

	// Clearing again stays idempotent.
	require.NoError(t, SaveProviderURLGroupName(urlKey, "", 11))

	var audits []ProviderBillingAudit
	require.NoError(t, db.Where("entity_type = ?", providerURLGroupNameEntity).Order("id").Find(&audits).Error)
	require.Len(t, audits, 3, "create, update and clear are audited; no-ops are not")
	assert.Equal(t, "create", audits[0].Action)
	assert.Equal(t, "update", audits[1].Action)
	assert.Equal(t, urlKey, audits[1].EntityKey)
	assert.Equal(t, 10, audits[1].OperatorId)
	assert.Equal(t, "delete", audits[2].Action)
	assert.Equal(t, 11, audits[2].OperatorId)
}

func TestSaveProviderURLGroupNameRejectsInvalidInput(t *testing.T) {
	setupBillingReconciliationTestDB(t)
	require.Error(t, SaveProviderURLGroupName("junk", "name", 9))
	require.Error(t, SaveProviderURLGroupName("", "name", 9))
	require.Error(t, SaveProviderURLGroupName("https://api.example.com", strings.Repeat("长", MaxProviderURLGroupNameLength+1), 9))

	var rows int64
	require.NoError(t, DB.Model(&ProviderURLGroupDisplayName{}).Count(&rows).Error)
	assert.Zero(t, rows, "rejected writes must not persist anything")
}

func TestSaveProviderURLGroupNameAcceptsFallbackChannelKeys(t *testing.T) {
	setupBillingReconciliationTestDB(t)
	require.NoError(t, SaveProviderURLGroupName("channel:24", "Legacy host", 9))
	names, err := getProviderURLGroupNames()
	require.NoError(t, err)
	assert.Equal(t, "Legacy host", names["channel:24"])
}

// A saved alias becomes the card title projection but never masks the
// unidentified or deleted markers of a per-channel fallback group.
func TestGetProviderBillingURLSummaryProjectsCustomNameWithoutMaskingState(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 21, Name: "alpha", BaseURL: urlPtr("https://api.example.com")},
		{Id: 25, Name: "broken", BaseURL: urlPtr("")},
	}).Error)
	require.NoError(t, SaveProviderURLGroupName("https://api.example.com", "Named upstream", 9))
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "m", PromptTokens: 10, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1310, Type: LogTypeConsume, ChannelId: 25, ModelName: "m", PromptTokens: 10},
	}).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 2)

	named := summary.Groups[0]
	assert.Equal(t, "https://api.example.com", named.UrlKey)
	assert.Equal(t, "Named upstream", named.CustomName)
	assert.False(t, named.Unidentified)

	// Sorting follows the effective label; the fallback group keeps its
	// channel fallback name and its unidentified marker.
	fallback := summary.Groups[1]
	assert.Equal(t, "channel:25", fallback.UrlKey)
	assert.Empty(t, fallback.CustomName)
	assert.True(t, fallback.Unidentified)
	assert.False(t, fallback.Deleted)

	// After clearing, the group falls back to the safe URL projection.
	require.NoError(t, SaveProviderURLGroupName("https://api.example.com", "", 9))
	summary, err = GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 2)
	assert.Empty(t, summary.Groups[0].CustomName)
	assert.Equal(t, "https://api.example.com", summary.Groups[0].DisplayName)
}

func TestSaveProviderURLGroupNameUnicodeLengthBoundary(t *testing.T) {
	for _, character := range []string{"A", "中", "😀"} {
		t.Run(character, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			name := strings.Repeat(character, 255)
			require.NoError(t, SaveProviderURLGroupName("channel:24", "  "+name+"  ", 9))
			require.EqualError(t, SaveProviderURLGroupName("channel:24", name+character, 9), "upstream name is too long")
			names, err := getProviderURLGroupNames()
			require.NoError(t, err)
			assert.Equal(t, name, names["channel:24"])
			var count int64
			require.NoError(t, db.Model(&ProviderBillingAudit{}).Count(&count).Error)
			assert.EqualValues(t, 1, count, "rejected names must not change the stored name or audit")
		})
	}
}
