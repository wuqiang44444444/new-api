package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type officialAssetFingerprintRecord struct {
	ChannelID   int
	Fingerprint string
	Source      string
}

func validateOfficialAssetCredentialMigration() error {
	requiredTables := []any{
		&AssetBinding{},
		&AssetGroupBinding{},
		&RealPersonAuthorization{},
		&AssetOwnershipClaim{},
		&AssetGroupOwnershipClaim{},
		&AssetReconciliationFinding{},
		&ChannelAssetCredential{},
	}
	for _, table := range requiredTables {
		if !DB.Migrator().HasTable(table) {
			return nil
		}
	}

	records := make([]officialAssetFingerprintRecord, 0)
	var bindings []AssetBinding
	if err := DB.Select("id", "channel_id", "credential_fingerprint").
		Where("upstream_profile = ? AND status <> ?", dto.AssetUpstreamProfileOfficial, AssetBindingStatusDeleted).
		Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: binding.ChannelID, Fingerprint: binding.CredentialFingerprint, Source: "asset binding",
		})
	}

	var groups []AssetGroupBinding
	if err := DB.Select("id", "channel_id", "credential_fingerprint").
		Where("upstream_profile = ? AND status <> ?", dto.AssetUpstreamProfileOfficial, AssetBindingStatusDeleted).
		Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: group.ChannelID, Fingerprint: group.CredentialFingerprint, Source: "asset group binding",
		})
	}

	var authorizations []RealPersonAuthorization
	if err := DB.Select("id", "channel_id", "credential_fingerprint").
		Where("upstream_profile = ? AND status NOT IN ?", dto.AssetUpstreamProfileOfficial, []string{
			RealPersonAuthorizationExpired,
			RealPersonAuthorizationRevoked,
			RealPersonAuthorizationDeleted,
		}).
		Find(&authorizations).Error; err != nil {
		return err
	}
	for _, authorization := range authorizations {
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: authorization.ChannelID, Fingerprint: authorization.CredentialFingerprint, Source: "real-person authorization",
		})
	}

	var findings []AssetReconciliationFinding
	if err := DB.Select("id", "channel_id", "credential_fingerprint").
		Where("upstream_profile = ? AND status = ?", dto.AssetUpstreamProfileOfficial, AssetReconciliationFindingOpen).
		Find(&findings).Error; err != nil {
		return err
	}
	for _, finding := range findings {
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: finding.ChannelID, Fingerprint: finding.CredentialFingerprint, Source: "reconciliation finding",
		})
	}

	var assetClaims []AssetOwnershipClaim
	if err := DB.Where("upstream_profile = ?", dto.AssetUpstreamProfileOfficial).Find(&assetClaims).Error; err != nil {
		return err
	}
	for _, claim := range assetClaims {
		var binding AssetBinding
		if err := DB.Select("channel_id").First(&binding, "id = ?", claim.AssetBindingID).Error; err != nil {
			return fmt.Errorf("official asset credential migration blocked by an orphan asset ownership claim")
		}
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: binding.ChannelID, Fingerprint: claim.ProviderAccountFingerprint, Source: "asset ownership claim",
		})
	}

	var groupClaims []AssetGroupOwnershipClaim
	if err := DB.Where("upstream_profile = ?", dto.AssetUpstreamProfileOfficial).Find(&groupClaims).Error; err != nil {
		return err
	}
	for _, claim := range groupClaims {
		var binding AssetGroupBinding
		if err := DB.Select("channel_id").First(&binding, "id = ?", claim.AssetGroupBindingID).Error; err != nil {
			return fmt.Errorf("official asset credential migration blocked by an orphan asset group ownership claim")
		}
		records = append(records, officialAssetFingerprintRecord{
			ChannelID: binding.ChannelID, Fingerprint: claim.ProviderAccountFingerprint, Source: "asset group ownership claim",
		})
	}

	channels := make(map[int]*Channel)
	missingChannels := make(map[int]bool)
	fingerprints := make(map[int]string)
	for _, record := range records {
		if missingChannels[record.ChannelID] {
			continue
		}
		channel := channels[record.ChannelID]
		if channel == nil {
			var err error
			channel, err = GetChannelById(record.ChannelID, true)
			if err != nil {
				// The referenced channel no longer exists, so these records are
				// orphans left behind by channel deletion, not official
				// credentials awaiting migration. Pruning them must never brick
				// gateway startup: drop the orphan reconciliation findings, warn
				// once per missing channel, and skip the remaining records.
				missingChannels[record.ChannelID] = true
				pruneOrphanOfficialReconciliationFindings(record.ChannelID)
				common.SysError(fmt.Sprintf("official asset credential migration: %s references unavailable channel %d; pruned orphan reconciliation findings", record.Source, record.ChannelID))
				continue
			}
			channels[record.ChannelID] = channel
		}
		fingerprint, ok := fingerprints[record.ChannelID]
		if !ok {
			_, currentFingerprint, err := ResolveAssetChannelCredential(channel)
			if err != nil {
				return fmt.Errorf("official asset credential migration blocked for channel %d: configure and verify the separate asset credential before upgrading existing records", record.ChannelID)
			}
			fingerprint = currentFingerprint
			fingerprints[record.ChannelID] = fingerprint
		}
		if record.Fingerprint != fingerprint {
			return fmt.Errorf("official asset credential migration blocked for channel %d: %s still uses a legacy or mismatched credential fingerprint", record.ChannelID, record.Source)
		}
	}
	return nil
}

func ValidateOfficialAssetCredentialMigration() error {
	return validateOfficialAssetCredentialMigration()
}

// pruneOrphanOfficialReconciliationFindings removes official reconciliation
// findings for a channel that no longer exists. Findings are diagnostic rows
// produced by reconciliation runs; once their channel is gone they have
// nothing left to reconcile and must not block startup.
func pruneOrphanOfficialReconciliationFindings(channelID int) {
	if err := DB.Where("channel_id = ? AND upstream_profile = ?", channelID, dto.AssetUpstreamProfileOfficial).
		Delete(&AssetReconciliationFinding{}).Error; err != nil {
		common.SysError(fmt.Sprintf("official asset credential migration: failed to prune orphan reconciliation findings for channel %d: %v", channelID, err))
	}
}
