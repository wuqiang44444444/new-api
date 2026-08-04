package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func updateChannelStatusTx(tx *gorm.DB, channelID int, usingKey string, status int, reason string, actorID int) (*Channel, bool, error) {
	var channel Channel
	if err := lockForUpdate(tx).First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, false, err
	}

	statusChanged := channel.Status != status
	if channel.ChannelInfo.IsMultiKey {
		beforeStatus := channel.Status
		handlerMultiKeyUpdate(&channel, usingKey, status, reason)
		statusChanged = beforeStatus != channel.Status
	} else {
		if !statusChanged {
			return &channel, false, nil
		}
		info := channel.GetOtherInfo()
		info["status_reason"] = reason
		info["status_time"] = common.GetTimestamp()
		channel.SetOtherInfo(info)
		channel.Status = status
	}

	if err := tx.Omit("key").Save(&channel).Error; err != nil {
		return nil, false, err
	}
	if statusChanged {
		if err := updateAbilityStatusTx(tx, &channel, status == common.ChannelStatusEnabled, actorID); err != nil {
			return nil, false, err
		}
	}
	return &channel, true, nil
}

func UpdateChannelStatusWithActor(channelID int, usingKey string, status int, reason string, actorID int) (bool, error) {
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()

	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		_, didChange, err := updateChannelStatusTx(tx, channelID, usingKey, status, reason, actorID)
		changed = didChange
		return err
	})
	if err != nil {
		return false, err
	}
	if changed && common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return changed, nil
}

func UpdateChannelStatusesWithActor(channelIDs []int, status int, reason string, actorID int) (int, error) {
	if len(channelIDs) == 0 {
		return 0, nil
	}
	changedCount := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, channelID := range channelIDs {
			_, changed, err := updateChannelStatusTx(tx, channelID, "", status, reason, actorID)
			if err != nil {
				return err
			}
			if changed {
				changedCount++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if changedCount > 0 && common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return changedCount, nil
}

func updateChannelsStatusByTag(tag string, status int, actorID int) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := lockForUpdate(tx).Where("tag = ?", tag).Find(&channels).Error; err != nil {
			return err
		}
		for i := range channels {
			if channels[i].Status == status {
				continue
			}
			channels[i].Status = status
			if err := tx.Model(&channels[i]).Select("status").Update("status", status).Error; err != nil {
				return err
			}
			if err := updateAbilityStatusTx(tx, &channels[i], status == common.ChannelStatusEnabled, actorID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return nil
}

func EnableChannelByTagWithActor(tag string, actorID int) error {
	return updateChannelsStatusByTag(tag, common.ChannelStatusEnabled, actorID)
}

func DisableChannelByTagWithActor(tag string, actorID int) error {
	return updateChannelsStatusByTag(tag, common.ChannelStatusManuallyDisabled, actorID)
}
