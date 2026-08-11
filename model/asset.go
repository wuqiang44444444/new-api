package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AssetKindGeneral    = "general"
	AssetKindRealPerson = "real_person"

	AssetStatusProcessing = "processing"
	AssetStatusReady      = "ready"
	AssetStatusFailed     = "failed"
	AssetStatusDeleted    = "deleted"
)

var (
	ErrAssetCountLimit                = errors.New("asset count limit reached")
	ErrChannelHasActiveAssetResources = errors.New("channel has active asset resources")
)

// Asset is the one-to-one platform projection of one Provider asset. The
// selected channel, credential scope and Provider resource never fan out.
type Asset struct {
	ID                     int64  `json:"-" gorm:"primaryKey"`
	PublicID               string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID                 int    `json:"-" gorm:"index:idx_assets_user_status"`
	CreatedByTokenID       int    `json:"-" gorm:"index"`
	AppID                  int    `json:"-" gorm:"index"`
	Name                   string `json:"name" gorm:"type:varchar(64)"`
	AssetKind              string `json:"asset_kind" gorm:"type:varchar(32);index"`
	MediaType              string `json:"media_type" gorm:"type:varchar(16);index"`
	RequestedModel         string `json:"-" gorm:"type:varchar(191);index"`
	ChannelID              int    `json:"-" gorm:"index"`
	CredentialFingerprint  string `json:"-" gorm:"type:varchar(64);index"`
	UpstreamProtocol       string `json:"-" gorm:"type:varchar(96);index"`
	ProviderProject        string `json:"-" gorm:"type:varchar(128)"`
	Region                 string `json:"-" gorm:"type:varchar(64)"`
	AssetGroupID           *int64 `json:"-" gorm:"index"`
	UpstreamResourceID     string `json:"-" gorm:"type:varchar(191)"`
	UpstreamBusinessID     string `json:"-" gorm:"type:varchar(191)"`
	UpstreamReferenceType  string `json:"-" gorm:"type:varchar(32)"`
	UpstreamReferenceValue string `json:"-" gorm:"type:varchar(512)"`
	Status                 string `json:"status" gorm:"type:varchar(32);index:idx_assets_user_status"`
	ErrorCode              string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage           string `json:"error,omitempty" gorm:"type:text"`
	CreatedAt              int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt              int64  `json:"updated_at" gorm:"bigint;index"`
	DeletedAt              int64  `json:"-" gorm:"bigint;index"`
}

// AssetGroup represents both ordinary Provider groups and the upstream
// real-person verification flow. The platform stores no face media.
type AssetGroup struct {
	ID                        int64  `json:"-" gorm:"primaryKey"`
	PublicID                  string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID                    int    `json:"-" gorm:"index"`
	CreatedByTokenID          int    `json:"-" gorm:"index"`
	AppID                     int    `json:"-" gorm:"index"`
	Name                      string `json:"name" gorm:"type:varchar(64)"`
	Description               string `json:"description,omitempty" gorm:"type:varchar(300)"`
	GroupKind                 string `json:"group_kind" gorm:"type:varchar(32);index"`
	RequestedModel            string `json:"-" gorm:"type:varchar(191);index"`
	ChannelID                 int    `json:"-" gorm:"index"`
	CredentialFingerprint     string `json:"-" gorm:"type:varchar(64);index"`
	UpstreamProtocol          string `json:"-" gorm:"type:varchar(96);index"`
	ProviderProject           string `json:"-" gorm:"type:varchar(128)"`
	Region                    string `json:"-" gorm:"type:varchar(64)"`
	UpstreamResourceID        string `json:"-" gorm:"type:varchar(191)"`
	UpstreamBusinessID        string `json:"-" gorm:"type:varchar(191)"`
	VerificationSessionID     string `json:"-" gorm:"type:varchar(191)"`
	VerificationURLCiphertext string `json:"-" gorm:"type:text"`
	VerificationURLExpiresAt  int64  `json:"-" gorm:"bigint"`
	Status                    string `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode                 string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage              string `json:"error,omitempty" gorm:"type:text"`
	CreatedAt                 int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                 int64  `json:"updated_at" gorm:"bigint;index"`
	DeletedAt                 int64  `json:"-" gorm:"bigint;index"`
}

type AssetListFilter struct {
	Status    string
	AssetKind string
	MediaType string
	Name      string
}

func (a *Asset) BeforeCreate(_ *gorm.DB) error {
	if a.PublicID == "" {
		id, err := generateAssetPublicID("ast_")
		if err != nil {
			return err
		}
		a.PublicID = id
	}
	now := common.GetTimestamp()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	return nil
}

func (g *AssetGroup) BeforeCreate(_ *gorm.DB) error {
	if g.PublicID == "" {
		id, err := generateAssetPublicID("astgrp_")
		if err != nil {
			return err
		}
		g.PublicID = id
	}
	now := common.GetTimestamp()
	if g.CreatedAt == 0 {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	return nil
}

func generateAssetPublicID(prefix string) (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return prefix + key, nil
}

func ValidateAssetKind(kind string) bool {
	return kind == AssetKindGeneral || kind == AssetKindRealPerson
}

func ValidateAssetMediaType(mediaType string) bool {
	switch mediaType {
	case "image", "video", "audio":
		return true
	default:
		return false
	}
}

func AssetCredentialFingerprint(baseURL, _ string, protocol string, providerScope ...string) string {
	// Credentials are intentionally excluded: rotating a Key/AK/SK on the same
	// channel must not invalidate resources already fixed to that channel and
	// Provider scope.
	input := strings.TrimRight(baseURL, "/") + "\n" + protocol
	for _, value := range providerScope {
		if strings.TrimSpace(value) != "" {
			input += "\n" + strings.TrimSpace(value)
		}
	}
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:])
}

func ChannelHasActiveAssetResources(channelID int) (bool, error) {
	if !DB.Migrator().HasTable(&Asset{}) {
		return false, nil
	}
	var count int64
	if err := DB.Model(&Asset{}).Where("channel_id = ? AND deleted_at = ?", channelID, 0).Count(&count).Error; err != nil || count > 0 {
		return count > 0, err
	}
	if !DB.Migrator().HasTable(&AssetGroup{}) {
		return false, nil
	}
	err := DB.Model(&AssetGroup{}).Where("channel_id = ? AND deleted_at = ?", channelID, 0).Count(&count).Error
	return count > 0, err
}

func FirstChannelWithActiveAssetResources(channelIDs []int) (int, bool, error) {
	for _, channelID := range channelIDs {
		hasActive, err := ChannelHasActiveAssetResources(channelID)
		if err != nil {
			return 0, false, err
		}
		if hasActive {
			return channelID, true, nil
		}
	}
	return 0, false, nil
}
