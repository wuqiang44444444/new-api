package model

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AssetReconciliationFindingOpen     = "open"
	AssetReconciliationFindingResolved = "resolved"

	AssetReconciliationOrphanUpstream  = "orphan_upstream"
	AssetReconciliationMissingUpstream = "missing_upstream"
)

type AssetReconciliationFinding struct {
	ID                    int64  `json:"-" gorm:"primaryKey"`
	ScopeHash             string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	ChannelID             int    `json:"-" gorm:"index"`
	CredentialFingerprint string `json:"-" gorm:"type:varchar(64);index"`
	UpstreamProfile       string `json:"-" gorm:"type:varchar(32)"`
	ProviderProject       string `json:"-" gorm:"type:varchar(128)"`
	Region                string `json:"-" gorm:"type:varchar(64)"`
	ResourceKind          string `json:"-" gorm:"type:varchar(20)"`
	UpstreamResourceID    string `json:"-" gorm:"type:varchar(191)"`
	FindingType           string `json:"-" gorm:"type:varchar(32);index"`
	Status                string `json:"-" gorm:"type:varchar(20);index"`
	FirstDetectedAt       int64  `json:"-" gorm:"bigint"`
	LastDetectedAt        int64  `json:"-" gorm:"bigint;index"`
	ResolvedAt            int64  `json:"-" gorm:"bigint"`
}

func NewAssetReconciliationFinding(channelID int, fingerprint, profile, project, region, resourceKind, resourceID, findingType string) AssetReconciliationFinding {
	scope := fmt.Sprintf(
		"%d\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
		channelID,
		strings.TrimSpace(fingerprint),
		strings.TrimSpace(profile),
		strings.TrimSpace(project),
		strings.TrimSpace(region),
		strings.TrimSpace(resourceKind),
		strings.TrimSpace(resourceID),
		strings.TrimSpace(findingType),
	)
	sum := sha256.Sum256([]byte(scope))
	now := common.GetTimestamp()
	return AssetReconciliationFinding{
		ScopeHash:             fmt.Sprintf("%x", sum[:]),
		ChannelID:             channelID,
		CredentialFingerprint: strings.TrimSpace(fingerprint),
		UpstreamProfile:       strings.TrimSpace(profile),
		ProviderProject:       strings.TrimSpace(project),
		Region:                strings.TrimSpace(region),
		ResourceKind:          strings.TrimSpace(resourceKind),
		UpstreamResourceID:    strings.TrimSpace(resourceID),
		FindingType:           strings.TrimSpace(findingType),
		Status:                AssetReconciliationFindingOpen,
		FirstDetectedAt:       now,
		LastDetectedAt:        now,
	}
}

func SaveAssetReconciliationFindings(tx *gorm.DB, channelID int, fingerprint string, findings []AssetReconciliationFinding) error {
	if tx == nil {
		tx = DB
	}
	now := common.GetTimestamp()
	seen := make([]string, 0, len(findings))
	for i := range findings {
		finding := findings[i]
		finding.LastDetectedAt = now
		if finding.FirstDetectedAt == 0 {
			finding.FirstDetectedAt = now
		}
		finding.Status = AssetReconciliationFindingOpen
		finding.ResolvedAt = 0
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "scope_hash"}},
			DoUpdates: clause.Assignments(map[string]any{
				"status":           AssetReconciliationFindingOpen,
				"last_detected_at": now,
				"resolved_at":      int64(0),
			}),
		}).Create(&finding).Error; err != nil {
			return err
		}
		seen = append(seen, finding.ScopeHash)
	}
	resolve := tx.Model(&AssetReconciliationFinding{}).
		Where("channel_id = ? AND credential_fingerprint = ? AND status = ?", channelID, fingerprint, AssetReconciliationFindingOpen)
	if len(seen) > 0 {
		resolve = resolve.Where("scope_hash NOT IN ?", seen)
	}
	return resolve.Updates(map[string]any{
		"status":      AssetReconciliationFindingResolved,
		"resolved_at": now,
	}).Error
}
