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

	AssetStatusCreating       = "creating"
	AssetStatusCreateUnknown  = "create_unknown"
	AssetStatusProcessing     = "processing"
	AssetStatusReady          = "ready"
	AssetStatusFailed         = "failed"
	AssetStatusDeleting       = "deleting"
	AssetStatusDeletionFailed = "deletion_failed"
	AssetStatusDeleted        = "deleted"

	AssetBindingStatusPending         = "pending"
	AssetBindingStatusCreating        = "creating"
	AssetBindingStatusCreateUnknown   = "create_unknown"
	AssetBindingStatusProcessing      = "processing"
	AssetBindingStatusActive          = "active"
	AssetBindingStatusFailed          = "failed"
	AssetBindingStatusStaleCredential = "stale_credential"
	AssetBindingStatusDeleting        = "deleting"
	AssetBindingStatusDeletionFailed  = "deletion_failed"
	AssetBindingStatusDeleted         = "deleted"
)

var (
	ErrAssetCountLimit = errors.New("asset count limit reached")
)

type Asset struct {
	ID                 int64  `json:"-" gorm:"primaryKey"`
	PublicID           string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID             int    `json:"-" gorm:"index:idx_assets_user_status"`
	CreatedByTokenID   int    `json:"-" gorm:"index"`
	AppID              int    `json:"-" gorm:"index"`
	EndUserSubjectHash string `json:"-" gorm:"type:varchar(64);index"`
	Name               string `json:"name" gorm:"type:varchar(64)"`
	AssetKind          string `json:"asset_kind" gorm:"type:varchar(32);index"`
	MediaType          string `json:"media_type" gorm:"type:varchar(16);index"`
	RequestedModel     string `json:"-" gorm:"type:varchar(191);index"`
	LinkPubSnapshot    `json:"-" gorm:"embedded"`
	AuthorizationID    *int64 `json:"-" gorm:"index"`
	SupersedesAssetID  *int64 `json:"-" gorm:"index"`
	MigrationBatchID   string `json:"-" gorm:"type:varchar(64);index"`
	MigrationReason    string `json:"-" gorm:"type:varchar(300)"`
	Status             string `json:"status" gorm:"type:varchar(32);index:idx_assets_user_status"`
	ErrorCode          string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage       string `json:"error,omitempty" gorm:"type:text"`
	CreatedAt          int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
	DeletedAt          int64  `json:"-" gorm:"bigint;index"`
}

type AssetBinding struct {
	ID                     int64  `json:"-" gorm:"primaryKey"`
	PublicID               string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	AssetID                int64  `json:"-" gorm:"uniqueIndex:idx_asset_channel_credential;index"`
	UserID                 int    `json:"-" gorm:"index"`
	ChannelID              int    `json:"-" gorm:"uniqueIndex:idx_asset_channel_credential;index"`
	CredentialFingerprint  string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_channel_credential"`
	LinkImplementationID   string `json:"-" gorm:"type:varchar(128);uniqueIndex:idx_asset_channel_credential;index"`
	LinkImplementationVer  string `json:"-" gorm:"column:link_implementation_version;type:varchar(32);uniqueIndex:idx_asset_channel_credential"`
	LinkImplementationHash string `json:"-" gorm:"type:varchar(80);index"`
	UpstreamProfile        string `json:"-" gorm:"type:varchar(32);index"`
	ProviderProject        string `json:"-" gorm:"type:varchar(128)"`
	Region                 string `json:"-" gorm:"type:varchar(64)"`
	UpstreamResourceID     string `json:"-" gorm:"type:varchar(191)"`
	UpstreamBusinessID     string `json:"-" gorm:"type:varchar(191)"`
	UpstreamRequestID      string `json:"-" gorm:"type:varchar(191);index"`
	UpstreamReferenceType  string `json:"-" gorm:"type:varchar(32)"`
	UpstreamReferenceValue string `json:"-" gorm:"type:varchar(512)"`
	UpstreamGroupBindingID *int64 `json:"-" gorm:"index"`
	RequestedModel         string `json:"model,omitempty" gorm:"type:varchar(191)"`
	LinkPubSnapshot        `json:"-" gorm:"embedded"`
	BindingTarget          string `json:"target,omitempty" gorm:"type:varchar(64)"`
	Status                 string `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode              string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage           string `json:"error,omitempty" gorm:"type:text"`
	CreatedAt              int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt              int64  `json:"updated_at" gorm:"bigint;index"`
}

type AssetGroupBinding struct {
	ID                    int64  `json:"-" gorm:"primaryKey"`
	UserID                int    `json:"-" gorm:"index"`
	AuthorizationID       *int64 `json:"-" gorm:"index"`
	ScopeKey              string `json:"-" gorm:"type:varchar(191);uniqueIndex:idx_asset_group_scope"`
	ChannelID             int    `json:"-" gorm:"uniqueIndex:idx_asset_group_scope;index"`
	CredentialFingerprint string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_group_scope"`
	UpstreamProfile       string `json:"-" gorm:"type:varchar(32)"`
	ProviderProject       string `json:"-" gorm:"type:varchar(128)"`
	Region                string `json:"-" gorm:"type:varchar(64)"`
	GroupKind             string `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_asset_group_scope"`
	Name                  string `json:"name" gorm:"type:varchar(64)"`
	Description           string `json:"description,omitempty" gorm:"type:varchar(300)"`
	UpstreamResourceID    string `json:"-" gorm:"type:varchar(191)"`
	UpstreamGroupID       string `json:"-" gorm:"type:varchar(191)"`
	UpstreamRequestID     string `json:"-" gorm:"type:varchar(191);index"`
	Status                string `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode             string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage          string `json:"error,omitempty" gorm:"type:text"`
	CreatedAt             int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt             int64  `json:"updated_at" gorm:"bigint;index"`
}

type AssetOwnershipClaim struct {
	ID                         int64  `json:"-" gorm:"primaryKey"`
	ProviderAccountFingerprint string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_ownership_scope"`
	UpstreamProfile            string `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_asset_ownership_scope"`
	ProviderProject            string `json:"-" gorm:"type:varchar(128);uniqueIndex:idx_asset_ownership_scope"`
	Region                     string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_ownership_scope"`
	UpstreamResourceID         string `json:"-" gorm:"type:varchar(191);uniqueIndex:idx_asset_ownership_scope"`
	AssetBindingID             int64  `json:"-" gorm:"uniqueIndex"`
	UserID                     int    `json:"-" gorm:"index"`
	CreatedAt                  int64  `json:"-" gorm:"bigint"`
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

func (b *AssetBinding) BeforeCreate(_ *gorm.DB) error {
	if b.PublicID == "" {
		id, err := generateAssetPublicID("ab_")
		if err != nil {
			return err
		}
		b.PublicID = id
	}
	now := common.GetTimestamp()
	if b.CreatedAt == 0 {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	return nil
}

func (g *AssetGroupBinding) BeforeCreate(_ *gorm.DB) error {
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

func GetAssetByPublicID(userID int, publicID string) (*Asset, error) {
	var asset Asset
	err := DB.Where("user_id = ? AND public_id = ? AND deleted_at = ?", userID, strings.TrimSpace(publicID), 0).First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	projected := []Asset{asset}
	if err := ProjectAssetStatuses(projected, common.GetTimestamp()); err != nil {
		return nil, err
	}
	return &projected[0], nil
}

func ListAssetsByUser(userID, offset, limit int, filters ...AssetListFilter) ([]Asset, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter := AssetListFilter{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	return listAssetsWithProjectedStatus(DB.Model(&Asset{}).Where("user_id = ? AND deleted_at = ?", userID, 0), offset, limit, filter)
}

func countUserAssets(tx *gorm.DB, userID int) (int64, error) {
	query := tx.Model(&Asset{}).Where("user_id = ? AND deleted_at = ?", userID, 0)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func ListAssetBindings(userID int, assetID int64) ([]AssetBinding, error) {
	var bindings []AssetBinding
	err := DB.Where("user_id = ? AND asset_id = ?", userID, assetID).Order("id desc").Find(&bindings).Error
	return bindings, err
}

func LoadAssetsForReference(userID int, publicIDs []string) ([]Asset, error) {
	if len(publicIDs) == 0 {
		return nil, nil
	}
	var assets []Asset
	if err := DB.Where("user_id = ? AND public_id IN ? AND deleted_at = ?", userID, publicIDs, 0).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(publicIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := ProjectAssetStatuses(assets, common.GetTimestamp()); err != nil {
		return nil, err
	}
	return assets, nil
}

func ActiveBindingsForAssets(assetIDs []int64) ([]AssetBinding, error) {
	var bindings []AssetBinding
	err := DB.Where("asset_id IN ? AND status = ? AND upstream_reference_type = ?", assetIDs, AssetBindingStatusActive, "asset_uri_id").Find(&bindings).Error
	return bindings, err
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

func AssetScopeKey(userID int, authorizationID *int64) string {
	if authorizationID != nil {
		return fmt.Sprintf("rpa:%d", *authorizationID)
	}
	return fmt.Sprintf("usr:%d", userID)
}

func AssetCredentialFingerprint(baseURL, key, profile string, providerScope ...string) string {
	input := strings.TrimRight(baseURL, "/") + "\n" + key + "\n" + profile
	for _, value := range providerScope {
		if strings.TrimSpace(value) != "" {
			input += "\n" + strings.TrimSpace(value)
		}
	}
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:])
}

func ChannelHasActiveAssetResources(channelID int) (bool, error) {
	// A pre-asset-schema database cannot contain an asset dependency.
	if !DB.Migrator().HasTable(&AssetBinding{}) {
		return false, nil
	}
	var bindingCount int64
	if err := DB.Model(&AssetBinding{}).Where("channel_id = ? AND status <> ?", channelID, AssetBindingStatusDeleted).Count(&bindingCount).Error; err != nil {
		return false, err
	}
	if bindingCount > 0 {
		return true, nil
	}
	var groupCount int64
	if err := DB.Model(&AssetGroupBinding{}).Where("channel_id = ? AND status <> ?", channelID, AssetBindingStatusDeleted).Count(&groupCount).Error; err != nil {
		return false, err
	}
	if groupCount > 0 {
		return true, nil
	}
	var authorizationCount int64
	err := DB.Model(&RealPersonAuthorization{}).Where("channel_id = ? AND status NOT IN ?", channelID, []string{RealPersonAuthorizationExpired, RealPersonAuthorizationRevoked, RealPersonAuthorizationDeleted}).Count(&authorizationCount).Error
	return authorizationCount > 0, err
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

func GetDisabledChannelIDs() ([]int, error) {
	var ids []int
	err := DB.Model(&Channel{}).Where("status = ? OR status = ?", common.ChannelStatusAutoDisabled, common.ChannelStatusManuallyDisabled).Pluck("id", &ids).Error
	return ids, err
}
