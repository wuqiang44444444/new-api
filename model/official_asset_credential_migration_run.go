package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type OfficialAssetCredentialMigrationInput struct {
	ChannelID                      int    `json:"channel_id"`
	ModelAPIKey                    string `json:"model_api_key"`
	AssetAccessKeyID               string `json:"asset_access_key_id"`
	AssetSecretAccessKey           string `json:"asset_secret_access_key"`
	ExpectedProviderProject        string `json:"expected_provider_project"`
	ExpectedRegion                 string `json:"expected_region"`
	AcknowledgeSameProviderAccount bool   `json:"acknowledge_same_provider_account"`
}

type OfficialAssetCredentialMigrationResult struct {
	ChannelID                   int  `json:"channel_id"`
	LegacyRecords               int  `json:"legacy_records"`
	AssetBindings               int  `json:"asset_bindings"`
	AssetGroupBindings          int  `json:"asset_group_bindings"`
	RealPersonAuthorizations    int  `json:"real_person_authorizations"`
	AssetOwnershipClaims        int  `json:"asset_ownership_claims"`
	AssetGroupOwnershipClaims   int  `json:"asset_group_ownership_claims"`
	AssetReconciliationFindings int  `json:"asset_reconciliation_findings"`
	Applied                     bool `json:"applied"`
}

type officialAssetMigrationRecords struct {
	assetBindings      []AssetBinding
	groupBindings      []AssetGroupBinding
	authorizations     []RealPersonAuthorization
	assetClaims        []AssetOwnershipClaim
	groupClaims        []AssetGroupOwnershipClaim
	reconciliation     []AssetReconciliationFinding
	legacyFingerprint  string
	currentFingerprint string
}

func RunOfficialAssetCredentialMigration(input OfficialAssetCredentialMigrationInput, apply bool) (OfficialAssetCredentialMigrationResult, error) {
	result := OfficialAssetCredentialMigrationResult{ChannelID: input.ChannelID}
	if input.ChannelID <= 0 {
		return result, errors.New("channel_id is required")
	}
	if apply && !input.AcknowledgeSameProviderAccount {
		return result, errors.New("same Provider account acknowledgement is required")
	}

	modelAPIKey := strings.TrimSpace(input.ModelAPIKey)
	credential, err := NormalizeChannelAssetCredential(&dto.ChannelAssetCredentialInput{
		AccessKeyID:     input.AssetAccessKeyID,
		SecretAccessKey: input.AssetSecretAccessKey,
	})
	if err != nil {
		return result, err
	}
	if modelAPIKey == "" {
		return result, errors.New("model_api_key is required")
	}
	if strings.Contains(modelAPIKey, "|") {
		return result, errors.New("model_api_key must not contain the asset credential separator")
	}

	run := func(tx *gorm.DB) error {
		var channel *Channel
		var err error
		if apply {
			channel, err = lockAssetLifecycleChannel(tx, input.ChannelID)
		} else {
			var stored Channel
			err = tx.First(&stored, "id = ?", input.ChannelID).Error
			channel = &stored
		}
		if err != nil {
			return err
		}
		if channel.Type != constant.ChannelTypeDoubaoVideo || channel.ChannelInfo.IsMultiKey {
			return errors.New("migration requires a single-key DoubaoVideo channel")
		}
		settings := channel.GetOtherSettings()
		if settings.AssetUpstreamProfile != dto.AssetUpstreamProfileOfficial {
			return errors.New("migration requires official_action_assets")
		}
		if strings.TrimSpace(settings.AssetProviderProject) != strings.TrimSpace(input.ExpectedProviderProject) {
			return errors.New("provider project does not match the expected value")
		}
		if strings.TrimSpace(settings.AssetRegion) != strings.TrimSpace(input.ExpectedRegion) {
			return errors.New("provider region does not match the expected value")
		}

		legacyKey := strings.TrimSpace(credential.AccessKeyID) + "|" + strings.TrimSpace(credential.SecretAccessKey)
		storedKey := strings.TrimSpace(channel.Key)
		if storedKey != legacyKey && storedKey != modelAPIKey {
			return errors.New("stored channel key does not match the supplied legacy AK/SK or model API key")
		}
		existingCredential, err := getChannelAssetCredential(tx, channel.Id)
		if err != nil {
			return err
		}
		if existingCredential != nil &&
			(strings.TrimSpace(existingCredential.AccessKeyID) != credential.AccessKeyID ||
				strings.TrimSpace(existingCredential.SecretAccessKey) != credential.SecretAccessKey) {
			return errors.New("stored asset credential does not match the supplied AK/SK")
		}

		records, err := loadOfficialAssetMigrationRecords(tx, channel, credential)
		if err != nil {
			return err
		}
		result.LegacyRecords = countLegacyOfficialAssetRecords(records)
		result.AssetBindings = len(records.assetBindings)
		result.AssetGroupBindings = len(records.groupBindings)
		result.RealPersonAuthorizations = len(records.authorizations)
		result.AssetOwnershipClaims = len(records.assetClaims)
		result.AssetGroupOwnershipClaims = len(records.groupClaims)
		result.AssetReconciliationFindings = len(records.reconciliation)
		if !apply {
			return nil
		}

		credential.ChannelID = channel.Id
		if err := saveChannelAssetCredentialTx(tx, credential); err != nil {
			return err
		}
		if err := updateOfficialAssetMigrationRecords(tx, records); err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Model(&Channel{}).
			Where("id = ?", channel.Id).
			Update("key", modelAPIKey).Error; err != nil {
			return err
		}
		channel.Key = modelAPIKey
		result.Applied = true
		return nil
	}

	if apply {
		err = DB.Transaction(run)
	} else {
		err = run(DB)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func loadOfficialAssetMigrationRecords(tx *gorm.DB, channel *Channel, credential *ChannelAssetCredential) (officialAssetMigrationRecords, error) {
	settings := channel.GetOtherSettings()
	legacyKey := strings.TrimSpace(credential.AccessKeyID) + "|" + strings.TrimSpace(credential.SecretAccessKey)
	records := officialAssetMigrationRecords{
		legacyFingerprint: AssetCredentialFingerprint(
			channel.GetBaseURL(),
			legacyKey,
			string(dto.AssetUpstreamProfileOfficial),
			settings.AssetProviderProject,
			settings.AssetRegion,
		),
		currentFingerprint: OfficialAssetCredentialFingerprint(
			credential.AccessKeyID,
			credential.SecretAccessKey,
			settings.AssetProviderProject,
			settings.AssetRegion,
		),
	}
	if err := tx.Where("channel_id = ? AND upstream_profile = ?", channel.Id, dto.AssetUpstreamProfileOfficial).
		Find(&records.assetBindings).Error; err != nil {
		return records, err
	}
	if err := tx.Where("channel_id = ? AND upstream_profile = ?", channel.Id, dto.AssetUpstreamProfileOfficial).
		Find(&records.groupBindings).Error; err != nil {
		return records, err
	}
	if err := tx.Where("channel_id = ? AND upstream_profile = ?", channel.Id, dto.AssetUpstreamProfileOfficial).
		Find(&records.authorizations).Error; err != nil {
		return records, err
	}
	if err := tx.Where("channel_id = ? AND upstream_profile = ?", channel.Id, dto.AssetUpstreamProfileOfficial).
		Find(&records.reconciliation).Error; err != nil {
		return records, err
	}

	assetBindingIDs := make([]int64, 0, len(records.assetBindings))
	for _, binding := range records.assetBindings {
		assetBindingIDs = append(assetBindingIDs, binding.ID)
	}
	if len(assetBindingIDs) > 0 {
		if err := tx.Where("asset_binding_id IN ? AND upstream_profile = ?", assetBindingIDs, dto.AssetUpstreamProfileOfficial).
			Find(&records.assetClaims).Error; err != nil {
			return records, err
		}
	}
	groupBindingIDs := make([]int64, 0, len(records.groupBindings))
	for _, binding := range records.groupBindings {
		groupBindingIDs = append(groupBindingIDs, binding.ID)
	}
	if len(groupBindingIDs) > 0 {
		if err := tx.Where("asset_group_binding_id IN ? AND upstream_profile = ?", groupBindingIDs, dto.AssetUpstreamProfileOfficial).
			Find(&records.groupClaims).Error; err != nil {
			return records, err
		}
	}
	if err := validateOfficialAssetMigrationFingerprints(records); err != nil {
		return records, err
	}
	return records, nil
}

func validateOfficialAssetMigrationFingerprints(records officialAssetMigrationRecords) error {
	check := func(source string, id int64, fingerprint string) error {
		if fingerprint == records.legacyFingerprint || fingerprint == records.currentFingerprint {
			return nil
		}
		return fmt.Errorf("%s %d has an unexpected credential fingerprint", source, id)
	}
	for _, record := range records.assetBindings {
		if err := check("asset binding", record.ID, record.CredentialFingerprint); err != nil {
			return err
		}
	}
	for _, record := range records.groupBindings {
		if err := check("asset group binding", record.ID, record.CredentialFingerprint); err != nil {
			return err
		}
	}
	for _, record := range records.authorizations {
		if err := check("real-person authorization", record.ID, record.CredentialFingerprint); err != nil {
			return err
		}
	}
	for _, record := range records.assetClaims {
		if err := check("asset ownership claim", record.ID, record.ProviderAccountFingerprint); err != nil {
			return err
		}
	}
	for _, record := range records.groupClaims {
		if err := check("asset group ownership claim", record.ID, record.ProviderAccountFingerprint); err != nil {
			return err
		}
	}
	for _, record := range records.reconciliation {
		if err := check("asset reconciliation finding", record.ID, record.CredentialFingerprint); err != nil {
			return err
		}
	}
	return nil
}

func countLegacyOfficialAssetRecords(records officialAssetMigrationRecords) int {
	count := 0
	for _, record := range records.assetBindings {
		if record.CredentialFingerprint == records.legacyFingerprint {
			count++
		}
	}
	for _, record := range records.groupBindings {
		if record.CredentialFingerprint == records.legacyFingerprint {
			count++
		}
	}
	for _, record := range records.authorizations {
		if record.CredentialFingerprint == records.legacyFingerprint {
			count++
		}
	}
	for _, record := range records.assetClaims {
		if record.ProviderAccountFingerprint == records.legacyFingerprint {
			count++
		}
	}
	for _, record := range records.groupClaims {
		if record.ProviderAccountFingerprint == records.legacyFingerprint {
			count++
		}
	}
	for _, record := range records.reconciliation {
		if record.CredentialFingerprint == records.legacyFingerprint {
			count++
		}
	}
	return count
}

func updateOfficialAssetMigrationRecords(tx *gorm.DB, records officialAssetMigrationRecords) error {
	assetBindingIDs := make([]int64, 0, len(records.assetBindings))
	for _, record := range records.assetBindings {
		assetBindingIDs = append(assetBindingIDs, record.ID)
	}
	if len(assetBindingIDs) > 0 {
		if err := tx.Model(&AssetBinding{}).Where("id IN ?", assetBindingIDs).
			Update("credential_fingerprint", records.currentFingerprint).Error; err != nil {
			return err
		}
	}
	groupBindingIDs := make([]int64, 0, len(records.groupBindings))
	for _, record := range records.groupBindings {
		groupBindingIDs = append(groupBindingIDs, record.ID)
	}
	if len(groupBindingIDs) > 0 {
		if err := tx.Model(&AssetGroupBinding{}).Where("id IN ?", groupBindingIDs).
			Update("credential_fingerprint", records.currentFingerprint).Error; err != nil {
			return err
		}
	}
	authorizationIDs := make([]int64, 0, len(records.authorizations))
	for _, record := range records.authorizations {
		authorizationIDs = append(authorizationIDs, record.ID)
	}
	if len(authorizationIDs) > 0 {
		if err := tx.Model(&RealPersonAuthorization{}).Where("id IN ?", authorizationIDs).
			Update("credential_fingerprint", records.currentFingerprint).Error; err != nil {
			return err
		}
	}
	assetClaimIDs := make([]int64, 0, len(records.assetClaims))
	for _, record := range records.assetClaims {
		assetClaimIDs = append(assetClaimIDs, record.ID)
	}
	if len(assetClaimIDs) > 0 {
		if err := tx.Model(&AssetOwnershipClaim{}).Where("id IN ?", assetClaimIDs).
			Update("provider_account_fingerprint", records.currentFingerprint).Error; err != nil {
			return err
		}
	}
	groupClaimIDs := make([]int64, 0, len(records.groupClaims))
	for _, record := range records.groupClaims {
		groupClaimIDs = append(groupClaimIDs, record.ID)
	}
	if len(groupClaimIDs) > 0 {
		if err := tx.Model(&AssetGroupOwnershipClaim{}).Where("id IN ?", groupClaimIDs).
			Update("provider_account_fingerprint", records.currentFingerprint).Error; err != nil {
			return err
		}
	}
	for _, finding := range records.reconciliation {
		updated := NewAssetReconciliationFinding(
			finding.ChannelID,
			records.currentFingerprint,
			finding.UpstreamProfile,
			finding.ProviderProject,
			finding.Region,
			finding.ResourceKind,
			finding.UpstreamResourceID,
			finding.FindingType,
		)
		if err := tx.Model(&AssetReconciliationFinding{}).Where("id = ?", finding.ID).Updates(map[string]any{
			"credential_fingerprint": records.currentFingerprint,
			"scope_hash":             updated.ScopeHash,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
