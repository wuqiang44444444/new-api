package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLinkPublicationTestDB(t *testing.T) {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	originalCapabilities := make(map[string]VideoSKUCapability, 2)
	originalImplementationHashes := make(map[string]string, 2)
	for _, publicModel := range []string{VideoSKUSeedance20Standard720P, VideoSKUSeedance20Value720P} {
		original := videoSKUCapabilities[publicModel]
		originalCapabilities[publicModel] = original
		originalImplementationHashes[publicModel] = videoSKUImplementationHashes[publicModel]
		verified := original
		verified.Ratios = []string{"16:9"}
		verified.ContentHash = videoSKUCapabilityHash(verified)
		videoSKUCapabilities[publicModel] = verified
		videoSKUImplementationHashes[publicModel] = verified.ContentHash
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &LinkModelPublication{}, &LinkModelPublicationAudit{}))
	t.Cleanup(func() {
		for publicModel, capability := range originalCapabilities {
			videoSKUCapabilities[publicModel] = capability
			videoSKUImplementationHashes[publicModel] = originalImplementationHashes[publicModel]
		}
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
	})
}

func feicaiAliasChannel(customerModel string) *Channel {
	channel := &Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: customerModel, Group: "default",
		Status:       common.ChannelStatusEnabled,
		ModelMapping: common.GetPointer(`{"` + customerModel + `":"seedance-2.0-vip-720p-azhw-feicai"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamCreatePath:        "/v1/videos",
		VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
		},
	})
	return channel
}

func TestLinkModelPublicationSurvivesCandidateRemovalAndRequiresExplicitRebind(t *testing.T) {
	setupLinkPublicationTestDB(t)
	channel := feicaiAliasChannel("customer-seedance")
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, EnsureChannelLinkModelPublications(DB, channel, 11))

	publication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	require.NoError(t, err)
	assert.Equal(t, VideoSKUSeedance20Standard720P, publication.LinkSKU)
	assert.EqualValues(t, 1, publication.PublicationVersion)

	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error)
	require.NoError(t, DB.Delete(channel).Error)
	publication, err = GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	require.NoError(t, err)
	assert.Equal(t, VideoSKUSeedance20Standard720P, publication.LinkSKU)

	_, err = EnsureLinkModelPublication(DB, LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-seedance",
	}, VideoSKUSeedance20Value720P, 12, 0, "implicit change")
	require.ErrorIs(t, err, ErrLinkModelPublicationConflict)

	publication, err = RebindLinkModelPublication(LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-seedance",
	}, VideoSKUSeedance20Value720P, 1, 12, "operator-approved contract migration")
	require.NoError(t, err)
	assert.Equal(t, VideoSKUSeedance20Value720P, publication.LinkSKU)
	assert.EqualValues(t, 2, publication.PublicationVersion)
	var auditCount int64
	require.NoError(t, DB.Model(&LinkModelPublicationAudit{}).Where("publication_id = ?", publication.ID).Count(&auditCount).Error)
	assert.EqualValues(t, 2, auditCount)
	var audits []LinkModelPublicationAudit
	require.NoError(t, DB.Where("publication_id = ?", publication.ID).Order("publication_version asc").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Empty(t, audits[0].PreviousLinkSKU)
	assert.Equal(t, VideoSKUSeedance20Standard720P, audits[0].LinkSKU)
	assert.Equal(t, VideoSKUSeedance20Standard720P, audits[1].PreviousLinkSKU)
	assert.Equal(t, VideoSKUSeedance20Value720P, audits[1].LinkSKU)
	assert.Equal(t, 12, audits[1].ChangedBy)
	assert.Equal(t, "operator-approved contract migration", audits[1].Reason)
	_, err = RebindLinkModelPublication(LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-seedance",
	}, VideoSKUSeedance20Standard720P, 1, 12, "stale operator request")
	assert.ErrorIs(t, err, ErrLinkModelPublicationVersionConflict)
	_, err = RebindLinkModelPublication(LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-seedance",
	}, "unregistered-sku", 2, 12, "invalid operator request")
	assert.ErrorIs(t, err, ErrLinkModelPublicationInvalidRebind)
}

func TestLinkChannelInsertPublishesAbilityAtomically(t *testing.T) {
	setupLinkPublicationTestDB(t)
	channel := feicaiAliasChannel("customer-seedance")
	require.NoError(t, channel.Insert())

	publication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	require.NoError(t, err)
	assert.Equal(t, VideoSKUSeedance20Standard720P, publication.LinkSKU)
	var abilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ? AND enabled = ?", channel.Id, "customer-seedance", true).Count(&abilityCount).Error)
	assert.EqualValues(t, 1, abilityCount)
	availability, err := GetLinkModelPublicationAvailability(publication, "default")
	require.NoError(t, err)
	assert.True(t, availability.CurrentlyFulfillable)
	assert.False(t, availability.RoutingConflict)

	conflicting := feicaiAliasChannel("customer-seedance")
	conflicting.ModelMapping = common.GetPointer(`{"customer-seedance":"seedance-2.0-vip-720p-mini-azhw-feicai"}`)
	require.ErrorContains(t, conflicting.Insert(), "non-equivalent channel")
	var persisted Channel
	assert.ErrorIs(t, DB.First(&persisted, "id = ?", conflicting.Id).Error, gorm.ErrRecordNotFound)
}

func TestLinkPublicationAvailabilityBatchAlignsResultsWithPublications(t *testing.T) {
	setupLinkPublicationTestDB(t)
	first := feicaiAliasChannel("customer-one")
	second := feicaiAliasChannel("customer-two")
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())
	require.NoError(t, DB.Where("channel_id = ?", second.Id).Delete(&Ability{}).Error)

	firstPublication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-one")
	require.NoError(t, err)
	secondPublication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-two")
	require.NoError(t, err)
	availabilities, err := GetLinkModelPublicationAvailabilities([]LinkModelPublication{*firstPublication, *secondPublication}, "default")
	require.NoError(t, err)
	require.Len(t, availabilities, 2)
	assert.True(t, availabilities[0].CurrentlyFulfillable)
	assert.False(t, availabilities[0].RoutingConflict)
	assert.False(t, availabilities[1].CurrentlyFulfillable)
	assert.False(t, availabilities[1].RoutingConflict)
}

func TestLinkChannelEnablePublishesAtomicallyWithOperator(t *testing.T) {
	setupLinkPublicationTestDB(t)
	channel := feicaiAliasChannel("customer-seedance")
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.Insert())

	changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusEnabled, "manual operation", 77)
	require.NoError(t, err)
	assert.True(t, changed)

	var persisted Channel
	require.NoError(t, DB.First(&persisted, "id = ?", channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, persisted.Status)
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ? AND model = ?", channel.Id, "customer-seedance").Error)
	assert.True(t, ability.Enabled)
	publication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	require.NoError(t, err)
	assert.Equal(t, 77, publication.CreatedBy)
	var audit LinkModelPublicationAudit
	require.NoError(t, DB.First(&audit, "publication_id = ?", publication.ID).Error)
	assert.Equal(t, 77, audit.ChangedBy)
}

func TestLinkChannelEnableConflictRollsBackChannelAndAbility(t *testing.T) {
	setupLinkPublicationTestDB(t)
	channel := feicaiAliasChannel("customer-seedance")
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.Insert())
	_, err := EnsureLinkModelPublication(DB, LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-seedance",
	}, VideoSKUSeedance20Value720P, 12, 0, "existing contract")
	require.NoError(t, err)

	changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusEnabled, "manual operation", 77)
	require.ErrorIs(t, err, ErrLinkModelPublicationConflict)
	assert.False(t, changed)

	var persisted Channel
	require.NoError(t, DB.First(&persisted, "id = ?", channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ? AND model = ?", channel.Id, "customer-seedance").Error)
	assert.False(t, ability.Enabled)
}

func TestLinkChannelBatchEnableConflictRollsBackEveryChannel(t *testing.T) {
	setupLinkPublicationTestDB(t)
	first := feicaiAliasChannel("customer-one")
	first.Status = common.ChannelStatusManuallyDisabled
	second := feicaiAliasChannel("customer-two")
	second.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())
	_, err := EnsureLinkModelPublication(DB, LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-two",
	}, VideoSKUSeedance20Value720P, 12, 0, "existing contract")
	require.NoError(t, err)

	changedCount, err := UpdateChannelStatusesWithActor([]int{first.Id, second.Id}, common.ChannelStatusEnabled, "manual batch operation", 77)
	require.ErrorIs(t, err, ErrLinkModelPublicationConflict)
	assert.Zero(t, changedCount)
	for _, channelID := range []int{first.Id, second.Id} {
		var persisted Channel
		require.NoError(t, DB.First(&persisted, "id = ?", channelID).Error)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
		var enabledCount int64
		require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND enabled = ?", channelID, true).Count(&enabledCount).Error)
		assert.Zero(t, enabledCount)
	}
	_, err = GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-one")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestLinkChannelTagEnableConflictRollsBackEveryChannel(t *testing.T) {
	setupLinkPublicationTestDB(t)
	first := feicaiAliasChannel("customer-one")
	first.Status = common.ChannelStatusManuallyDisabled
	first.SetTag("link-tag")
	second := feicaiAliasChannel("customer-two")
	second.Status = common.ChannelStatusManuallyDisabled
	second.SetTag("link-tag")
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())
	_, err := EnsureLinkModelPublication(DB, LinkModelPublicationKey{
		RouteFamily: LinkRouteFamilyModelArkVideo, CustomerModel: "customer-two",
	}, VideoSKUSeedance20Value720P, 12, 0, "existing contract")
	require.NoError(t, err)

	err = EnableChannelByTagWithActor("link-tag", 77)
	require.ErrorIs(t, err, ErrLinkModelPublicationConflict)
	for _, channelID := range []int{first.Id, second.Id} {
		var persisted Channel
		require.NoError(t, DB.First(&persisted, "id = ?", channelID).Error)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
		var enabledCount int64
		require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND enabled = ?", channelID, true).Count(&enabledCount).Error)
		assert.Zero(t, enabledCount)
	}
}

func TestLinkChannelTagEditPublishesWithOperator(t *testing.T) {
	setupLinkPublicationTestDB(t)
	channel := feicaiAliasChannel("customer-seedance")
	channel.SetTag("link-tag")
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "customer-seedance", ChannelId: channel.Id, Enabled: true,
	}).Error)
	modelMapping := channel.GetModelMapping()

	require.NoError(t, EditChannelByTagWithActor(
		"link-tag", nil, &modelMapping, nil, nil, nil, nil, nil, nil, 88,
	))
	publication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	require.NoError(t, err)
	assert.Equal(t, 88, publication.CreatedBy)
}

func TestLinkPublicationRejectsOrdinaryChannelInSameScope(t *testing.T) {
	setupLinkPublicationTestDB(t)
	ordinary := &Channel{Type: constant.ChannelTypeDoubaoVideo, Models: "customer-seedance", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(ordinary).Error)
	require.NoError(t, DB.Create(&Ability{Group: "default", Model: "customer-seedance", ChannelId: ordinary.Id, Enabled: true}).Error)

	linkChannel := feicaiAliasChannel("customer-seedance")
	require.NoError(t, DB.Create(linkChannel).Error)
	err := EnsureChannelLinkModelPublications(DB, linkChannel, 11)
	require.ErrorContains(t, err, "conflicts with ordinary channel")
}

func TestDirectLinkPublicationMigrationIsConservative(t *testing.T) {
	setupLinkPublicationTestDB(t)
	direct := feicaiAliasChannel(VideoSKUSeedance20Standard720P)
	require.NoError(t, DB.Create(direct).Error)
	alias := feicaiAliasChannel("customer-seedance")
	require.NoError(t, DB.Create(alias).Error)
	invalid := feicaiAliasChannel(VideoSKUSeedance20Value720P)
	invalid.ModelMapping = common.GetPointer(`{"seedance-2.0-value-720p":"seedance-2.0-933-720p-azhw-feicai"}`)
	settings := invalid.GetOtherSettings()
	settings.VideoUpstreamCreatePath = "/wrong"
	invalid.SetOtherSettings(settings)
	require.NoError(t, DB.Create(invalid).Error)

	require.NoError(t, MigrateDirectLinkModelPublications())
	publication, err := GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, VideoSKUSeedance20Standard720P)
	require.NoError(t, err)
	assert.Equal(t, VideoSKUSeedance20Standard720P, publication.LinkSKU)
	_, err = GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, "customer-seedance")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = GetLinkModelPublication(LinkContractNamespaceDefault, LinkRouteFamilyModelArkVideo, VideoSKUSeedance20Value720P)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
