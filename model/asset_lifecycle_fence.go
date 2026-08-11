package model

import (
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrAssetChannelUnavailable       = errors.New("asset channel is unavailable")
	ErrAssetChannelCredentialChanged = errors.New("asset channel credential has changed")
)

func acquireSQLiteAssetLifecycleWriteLock(tx *gorm.DB, value any, id any) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}
	return tx.Model(value).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error
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

func channelHasActiveAssetResourcesTx(tx *gorm.DB, channelID int) (bool, error) {
	var count int64
	if tx.Migrator().HasTable(&Asset{}) {
		if err := tx.Model(&Asset{}).Where("channel_id = ? AND deleted_at = ?", channelID, 0).Count(&count).Error; err != nil || count > 0 {
			return count > 0, err
		}
	}
	if !tx.Migrator().HasTable(&AssetGroup{}) {
		return false, nil
	}
	err := tx.Model(&AssetGroup{}).Where("channel_id = ? AND deleted_at = ?", channelID, 0).Count(&count).Error
	return count > 0, err
}

func channelAssetIdentityChanged(tx *gorm.DB, current, update *Channel, credential *ChannelAssetCredential) (bool, error) {
	if current == nil || update == nil {
		return false, errors.New("channel is required")
	}
	effective := *current
	if update.Type != 0 {
		effective.Type = update.Type
	}
	if update.Key != "" {
		effective.Key = update.Key
		effective.Keys = nil
	}
	if update.BaseURL != nil {
		effective.BaseURL = update.BaseURL
	}
	if update.OtherSettings != "" {
		effective.OtherSettings = update.OtherSettings
	}
	currentFingerprint, currentErr := assetLifecycleChannelFingerprintTx(tx, current, nil)
	effectiveFingerprint, effectiveErr := assetLifecycleChannelFingerprintTx(tx, &effective, credential)
	if currentErr != nil || effectiveErr != nil {
		return current.Type != effective.Type || current.Key != effective.Key || current.OtherSettings != effective.OtherSettings, nil
	}
	return currentFingerprint != effectiveFingerprint, nil
}

func assetLifecycleChannelFingerprintTx(tx *gorm.DB, channel *Channel, credential *ChannelAssetCredential) (string, error) {
	_, fingerprint, err := resolveAssetChannelCredential(tx, channel, credential)
	return fingerprint, err
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
	rows, err := batchDeleteChannelsWithAssetFence([]int{channel.Id})
	if err != nil {
		return err
	}
	if rows == 0 {
		return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	}
	return nil
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
	var ids []int
	if err := DB.Model(&Channel{}).Where("status IN ?", statuses).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return batchDeleteChannelsWithAssetFence(ids)
}

func GetDisabledChannelIDs() ([]int, error) {
	var ids []int
	err := DB.Model(&Channel{}).Where("status != ?", common.ChannelStatusEnabled).Pluck("id", &ids).Error
	return ids, err
}

func deleteChannelRowsTx(tx *gorm.DB, ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := tx.Where("id IN ?", ids).Delete(&Channel{})
	return result.RowsAffected, result.Error
}

func deleteChannelAbilitiesTx(tx *gorm.DB, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
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
