package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AssetCreateIdempotencyCreating      = "creating"
	AssetCreateIdempotencyProcessing    = "processing"
	AssetCreateIdempotencyComplete      = "complete"
	AssetCreateIdempotencyCreateUnknown = "create_unknown"
	AssetCreateIdempotencyFailed        = "failed"
)

var (
	ErrAssetIdempotencyConflict        = errors.New("asset idempotency conflict")
	ErrAssetAuthorizationNotAuthorized = errors.New("real-person authorization is not authorized")
)

// AssetCreateIdempotency is deliberately scoped to remote asset creation. It
// stores only keyed digests and never stores the client URL or raw key.
type AssetCreateIdempotency struct {
	ID          int64  `json:"-" gorm:"primaryKey"`
	UserID      int    `json:"-" gorm:"uniqueIndex:idx_asset_create_idempotency_scope"`
	Endpoint    string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_create_idempotency_scope"`
	KeyHash     string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_create_idempotency_scope"`
	RequestHMAC string `json:"-" gorm:"type:varchar(64)"`
	AssetID     int64  `json:"-" gorm:"index"`
	Status      string `json:"-" gorm:"type:varchar(32);index"`
	ExpiresAt   int64  `json:"-" gorm:"bigint;index"`
	CreatedAt   int64  `json:"-" gorm:"bigint;index"`
	UpdatedAt   int64  `json:"-" gorm:"bigint;index"`
}

func (i *AssetCreateIdempotency) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if i.CreatedAt == 0 {
		i.CreatedAt = now
	}
	i.UpdatedAt = now
	return nil
}

func LoadRemoteAssetCreateReplay(idempotency *AssetCreateIdempotency) (*Asset, *AssetBinding, bool, error) {
	if idempotency == nil {
		return nil, nil, false, gorm.ErrRecordNotFound
	}
	var existing AssetCreateIdempotency
	err := DB.Where("user_id = ? AND endpoint = ? AND key_hash = ? AND expires_at > ?", idempotency.UserID, idempotency.Endpoint, idempotency.KeyHash, common.GetTimestamp()).First(&existing).Error
	if err != nil {
		return nil, nil, false, err
	}
	if existing.RequestHMAC != idempotency.RequestHMAC {
		return nil, nil, false, ErrAssetIdempotencyConflict
	}
	var existingAsset Asset
	if err := DB.First(&existingAsset, "id = ? AND user_id = ?", existing.AssetID, existing.UserID).Error; err != nil {
		return nil, nil, false, err
	}
	var existingBinding AssetBinding
	if err := DB.First(&existingBinding, "asset_id = ? AND user_id = ?", existing.AssetID, existing.UserID).Error; err != nil {
		return nil, nil, false, err
	}
	return &existingAsset, &existingBinding, true, nil
}

func CreateRemoteAssetWithQuota(asset *Asset, binding *AssetBinding, idempotency *AssetCreateIdempotency, maxCount, createUnknownTTLSeconds int64) (*Asset, *AssetBinding, bool, error) {
	if existingAsset, existingBinding, replay, err := LoadRemoteAssetCreateReplay(idempotency); err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return existingAsset, existingBinding, replay, err
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		// User is the first lifecycle lock on every create/delete path. This
		// prevents deletion from missing a just-created asset and keeps the
		// user -> authorization lock order consistent.
		if _, err := lockAssetLifecycleUser(tx, asset.UserID); err != nil {
			return err
		}
		if asset.AuthorizationID != nil {
			authorization, err := LockRealPersonAuthorization(tx, *asset.AuthorizationID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAssetAuthorizationNotAuthorized
				}
				return err
			}
			if authorization.UserID != asset.UserID || authorization.Status != RealPersonAuthorizationAuthorized || authorization.RevokedAt != 0 {
				return ErrAssetAuthorizationNotAuthorized
			}
		}
		if idempotency != nil {
			var existing AssetCreateIdempotency
			err := lockForUpdate(tx).Where("user_id = ? AND endpoint = ? AND key_hash = ?", idempotency.UserID, idempotency.Endpoint, idempotency.KeyHash).First(&existing).Error
			if err == nil {
				if existing.ExpiresAt > common.GetTimestamp() {
					if existing.RequestHMAC != idempotency.RequestHMAC {
						return ErrAssetIdempotencyConflict
					}
					return fmt.Errorf("existing asset idempotency row must be loaded before creation")
				}
				if err := tx.Delete(&existing).Error; err != nil {
					return err
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		// The selected channel can change after the cache/read-side selection.
		// Recheck it while holding the same row lock used by destructive channel
		// mutations; once this transaction commits, the new binding fences them.
		if err := validateAssetCreateChannel(tx, binding); err != nil {
			return err
		}
		count, err := countUserAssets(tx, asset.UserID)
		if err != nil {
			return err
		}
		if count >= maxCount {
			return ErrAssetCountLimit
		}
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		binding.AssetID = asset.ID
		if err := tx.Create(binding).Error; err != nil {
			return err
		}
		watchdog := &AssetOperationJob{
			IdempotencyKey: fmt.Sprintf("resolve-unknown-create:%d", binding.ID),
			Kind:           "resolve_unknown_create",
			AssetID:        &asset.ID,
			BindingID:      &binding.ID,
			NextAttemptAt:  common.GetTimestamp() + createUnknownTTLSeconds,
		}
		if _, err := EnsureAssetOperationJob(tx, watchdog, false); err != nil {
			return err
		}
		if idempotency != nil {
			idempotency.AssetID = asset.ID
			if err := tx.Create(idempotency).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return asset, binding, false, nil
	}
	if idempotency != nil && (isAssetUniqueConstraintError(err) || strings.Contains(err.Error(), "must be loaded before creation")) {
		if existingAsset, existingBinding, replay, lookupErr := LoadRemoteAssetCreateReplay(idempotency); lookupErr == nil || !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return existingAsset, existingBinding, replay, lookupErr
		}
	}
	return nil, nil, false, err
}

func isAssetUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
