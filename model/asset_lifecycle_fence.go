package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

var (
	ErrAssetChannelUnavailable        = errors.New("asset channel is unavailable")
	ErrAssetChannelCredentialChanged  = errors.New("asset channel credential has changed")
	ErrChannelHasActiveAssetResources = errors.New("channel has active asset resources")
)

// SQLite does not support SELECT ... FOR UPDATE. A no-op write acquires its
// database-wide writer lock, so two successful lifecycle transactions cannot
// pass the same fence concurrently. MySQL and PostgreSQL use row locks below.
func acquireSQLiteAssetLifecycleWriteLock(tx *gorm.DB, value any, id any) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}
	return tx.Model(value).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error
}

func lockAssetLifecycleUser(tx *gorm.DB, id int) (*User, error) {
	if err := acquireSQLiteAssetLifecycleWriteLock(tx, &User{}, id); err != nil {
		return nil, err
	}
	var user User
	if err := lockForUpdate(tx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func lockAssetLifecycleChannel(tx *gorm.DB, id int) (*Channel, error) {
	if err := acquireSQLiteAssetLifecycleWriteLock(tx, &Channel{}, id); err != nil {
		return nil, err
	}
	var channel Channel
	if err := lockForUpdate(tx).First(&channel, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func validateAssetCreateChannel(tx *gorm.DB, binding *AssetBinding) error {
	channel, err := lockAssetLifecycleChannel(tx, binding.ChannelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssetChannelUnavailable
		}
		return err
	}
	if channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeDoubaoVideo {
		return ErrAssetChannelUnavailable
	}
	requestedModel := strings.TrimSpace(binding.RequestedModel)
	if binding.LinkImplementationID != "" || IsRegisteredLinkSKU(requestedModel) {
		implementation, ok := ResolveChannelLinkImplementation(channel)
		if !ok || implementation.ID != binding.LinkImplementationID ||
			implementation.Version != binding.LinkImplementationVer ||
			implementation.ContentHash != binding.LinkImplementationHash {
			return ErrAssetChannelUnavailable
		}
	}
	if IsRegisteredLinkSKU(requestedModel) {
		if err := ValidateChannelLinkImplementationForSKU(channel, requestedModel); err != nil {
			return fmt.Errorf("%w: %v", ErrAssetChannelUnavailable, err)
		}
	}
	fingerprint, err := assetLifecycleChannelFingerprintTx(tx, channel, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAssetChannelUnavailable, err)
	}
	if fingerprint != binding.CredentialFingerprint {
		return ErrAssetChannelCredentialChanged
	}
	return nil
}

func assetLifecycleChannelFingerprintTx(tx *gorm.DB, channel *Channel, credential *ChannelAssetCredential) (string, error) {
	_, fingerprint, err := resolveAssetChannelCredential(tx, channel, credential)
	return fingerprint, err
}

func channelHasActiveAssetResourcesTx(tx *gorm.DB, channelID int) (bool, error) {
	// Databases created by older versions do not have asset bindings. In that
	// state there cannot be an asset lifecycle dependency to fence.
	if !tx.Migrator().HasTable(&AssetBinding{}) {
		return false, nil
	}
	var count int64
	if err := tx.Model(&AssetBinding{}).Where("channel_id = ? AND status <> ?", channelID, AssetBindingStatusDeleted).Count(&count).Error; err != nil || count > 0 {
		return count > 0, err
	}
	if err := tx.Model(&AssetGroupBinding{}).Where("channel_id = ? AND status <> ?", channelID, AssetBindingStatusDeleted).Count(&count).Error; err != nil || count > 0 {
		return count > 0, err
	}
	err := tx.Model(&RealPersonAuthorization{}).
		Where("channel_id = ? AND status NOT IN ?", channelID, []string{RealPersonAuthorizationExpired, RealPersonAuthorizationRevoked, RealPersonAuthorizationDeleted}).
		Count(&count).Error
	return count > 0, err
}

func channelAssetIdentityChanged(tx *gorm.DB, current, update *Channel, credential *ChannelAssetCredential) (bool, error) {
	if current == nil || update == nil {
		return false, errors.New("channel is required")
	}
	effective := *current
	changed := false
	if update.Type != 0 && update.Type != current.Type {
		effective.Type = update.Type
		return true, nil
	}
	if update.Key != "" && update.Key != current.Key {
		effective.Key = update.Key
		effective.Keys = nil
		return true, nil
	}
	if update.BaseURL != nil && (current.BaseURL == nil || *update.BaseURL != *current.BaseURL) {
		effective.BaseURL = update.BaseURL
		return true, nil
	}
	if update.OtherSettings != "" && update.OtherSettings != current.OtherSettings {
		effective.OtherSettings = update.OtherSettings
		changed = true
	}
	if credential != nil {
		changed = true
	}
	if !changed {
		return false, nil
	}
	currentFingerprint, err := assetLifecycleChannelFingerprintTx(tx, current, nil)
	if err != nil {
		return true, nil
	}
	effectiveFingerprint, err := assetLifecycleChannelFingerprintTx(tx, &effective, credential)
	if err != nil {
		return true, nil
	}
	return currentFingerprint != effectiveFingerprint, nil
}

func updateChannelWithAssetFence(channel *Channel, credential *ChannelAssetCredential) error {
	return updateChannelWithAssetFenceActor(channel, credential, 0)
}

func updateChannelWithAssetFenceActor(channel *Channel, credential *ChannelAssetCredential, actorID int) error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockAssetLifecycleChannel(tx, channel.Id)
		if err != nil {
			return err
		}
		identityChanged, err := channelAssetIdentityChanged(tx, current, channel, credential)
		if err != nil {
			return err
		}
		if identityChanged {
			active, err := channelHasActiveAssetResourcesTx(tx, channel.Id)
			if err != nil {
				return err
			}
			if active {
				return fmt.Errorf("%w: channel %d", ErrChannelHasActiveAssetResources, channel.Id)
			}
		}
		if err := tx.Model(current).Updates(channel).Error; err != nil {
			return err
		}
		if credential != nil {
			if err := saveChannelAssetCredentialTx(tx, credential); err != nil {
				return err
			}
		}
		if err := tx.First(channel, "id = ?", channel.Id).Error; err != nil {
			return err
		}
		return channel.UpdateAbilitiesWithActor(tx, actorID)
	})
}

func deleteChannelWithAssetFence(channel *Channel) error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockAssetLifecycleChannel(tx, channel.Id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
				return err
			}
			return deleteChannelAssetCredentialsTx(tx, []int{channel.Id})
		}
		if err != nil {
			return err
		}
		active, err := channelHasActiveAssetResourcesTx(tx, channel.Id)
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("%w: channel %d", ErrChannelHasActiveAssetResources, channel.Id)
		}
		if err := tx.Delete(current).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
			return err
		}
		if err := deleteChannelAssetCredentialsTx(tx, []int{channel.Id}); err != nil {
			return err
		}
		*channel = *current
		return nil
	})
}

func batchDeleteChannelsWithAssetFence(ids []int) (int64, error) {
	ids = sortedUniqueChannelIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	var rows int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		deletable := make([]int, 0, len(ids))
		for _, id := range ids {
			channel, err := lockAssetLifecycleChannel(tx, id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			active, err := channelHasActiveAssetResourcesTx(tx, channel.Id)
			if err != nil {
				return err
			}
			if active {
				return fmt.Errorf("%w: channel %d", ErrChannelHasActiveAssetResources, channel.Id)
			}
			deletable = append(deletable, channel.Id)
		}
		if len(deletable) == 0 {
			return nil
		}
		var err error
		rows, err = deleteChannelRowsTx(tx, deletable)
		if err != nil {
			return err
		}
		if err := deleteChannelAbilitiesTx(tx, deletable); err != nil {
			return err
		}
		return deleteChannelAssetCredentialsTx(tx, deletable)
	})
	return rows, err
}

func deleteChannelsByStatusWithAssetFence(statuses []int) (int64, error) {
	var rows int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var ids []int
		if err := tx.Model(&Channel{}).Where("status IN ?", statuses).Order("id asc").Pluck("id", &ids).Error; err != nil {
			return err
		}
		deletable := make([]int, 0, len(ids))
		for _, id := range ids {
			channel, err := lockAssetLifecycleChannel(tx, id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if !containsChannelStatus(statuses, channel.Status) {
				continue
			}
			active, err := channelHasActiveAssetResourcesTx(tx, channel.Id)
			if err != nil {
				return err
			}
			if active {
				return fmt.Errorf("%w: channel %d", ErrChannelHasActiveAssetResources, channel.Id)
			}
			deletable = append(deletable, channel.Id)
		}
		if len(deletable) == 0 {
			return nil
		}
		var err error
		rows, err = deleteChannelRowsTx(tx, deletable)
		if err != nil {
			return err
		}
		return deleteChannelAssetCredentialsTx(tx, deletable)
	})
	return rows, err
}

func deleteChannelRowsTx(tx *gorm.DB, ids []int) (int64, error) {
	var rows int64
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		result := tx.Where("id IN ?", ids[start:end]).Delete(&Channel{})
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	return rows, nil
}

func deleteChannelAbilitiesTx(tx *gorm.DB, ids []int) error {
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		if err := tx.Where("channel_id IN ?", ids[start:end]).Delete(&Ability{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func sortedUniqueChannelIDs(ids []int) []int {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			set[id] = struct{}{}
		}
	}
	result := make([]int, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}

func containsChannelStatus(statuses []int, status int) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}
