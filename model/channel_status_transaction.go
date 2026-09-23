package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func updateChannelStatusTx(tx *gorm.DB, channelID int, usingKey string, status int, reason string, actorID int, automatic bool) (*Channel, int, bool, error) {
	channel, err := lockChannelForMutation(tx, channelID)
	if err != nil {
		return nil, 0, false, err
	}

	beforeStatus := channel.Status
	if channel.Status == status && (automatic || usingKey == "") {
		return channel, beforeStatus, false, nil
	}
	statusChanged := channel.Status != status
	if channel.ChannelInfo.IsMultiKey {
		handlerMultiKeyUpdate(channel, usingKey, status, reason)
		statusChanged = beforeStatus != channel.Status
	} else {
		if !statusChanged {
			return channel, beforeStatus, false, nil
		}
		info := channel.GetOtherInfo()
		info["status_reason"] = reason
		info["status_time"] = common.GetTimestamp()
		channel.SetOtherInfo(info)
		channel.Status = status
	}

	fields := []string{"status", "other_info"}
	if channel.ChannelInfo.IsMultiKey {
		fields = append(fields, "channel_info")
	}
	if err := tx.Model(channel).Select(fields).Updates(channel).Error; err != nil {
		return nil, 0, false, err
	}
	if statusChanged {
		if err := updateAbilityStatusTx(tx, channel, channel.Status == common.ChannelStatusEnabled, actorID); err != nil {
			return nil, 0, false, err
		}
	}
	return channel, beforeStatus, true, nil
}

func UpdateChannelStatusWithActor(channelID int, usingKey string, status int, reason string, actorID int, audit ...ChannelStatusAudit) (bool, error) {
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()

	changed := false
	var beforeStatus, afterStatus int
	err := DB.Transaction(func(tx *gorm.DB) error {
		channel, before, didChange, err := updateChannelStatusTx(tx, channelID, usingKey, status, reason, actorID, false)
		beforeStatus = before
		if channel != nil {
			afterStatus = channel.Status
		}
		changed = didChange
		return err
	})
	if err != nil {
		return false, err
	}
	if changed {
		recordChannelStatusTransition(channelID, beforeStatus, afterStatus, ChannelStatusSourceManual, actorID, audit...)
	}
	if changed && common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return changed, nil
}

func UpdateChannelStatusesWithActor(channelIDs []int, status int, reason string, actorID int, audit ...ChannelStatusAudit) (int, error) {
	if len(channelIDs) == 0 {
		return 0, nil
	}
	changedCount := 0
	type channelStatusAuditRow struct {
		channelID int
		before    int
		after     int
	}
	var auditRows []channelStatusAuditRow
	err := DB.Transaction(func(tx *gorm.DB) error {
		auditRows = nil
		for _, channelID := range channelIDs {
			channel, before, changed, err := updateChannelStatusTx(tx, channelID, "", status, reason, actorID, false)
			if err != nil {
				return err
			}
			if changed {
				auditRows = append(auditRows, channelStatusAuditRow{channelID: channelID, before: before, after: channel.Status})
				changedCount++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, row := range auditRows {
		recordChannelStatusTransition(row.channelID, row.before, row.after, ChannelStatusSourceManualBatch, actorID, audit...)
	}
	if changedCount > 0 && common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return changedCount, nil
}

func updateChannelsStatusByTag(tag string, status int, actorID int, audit ...ChannelStatusAudit) error {
	type channelStatusAuditRow struct {
		channelID int
		before    int
		after     int
	}
	var auditRows []channelStatusAuditRow
	err := DB.Transaction(func(tx *gorm.DB) error {
		auditRows = nil
		var channels []Channel
		if err := lockForUpdate(tx).Where("tag = ?", tag).Find(&channels).Error; err != nil {
			return err
		}
		for i := range channels {
			if channels[i].Status == status {
				continue
			}
			beforeStatus := channels[i].Status
			channels[i].Status = status
			if err := tx.Model(&channels[i]).Select("status").Update("status", status).Error; err != nil {
				return err
			}
			if err := updateAbilityStatusTx(tx, &channels[i], status == common.ChannelStatusEnabled, actorID); err != nil {
				return err
			}
			auditRows = append(auditRows, channelStatusAuditRow{channelID: channels[i].Id, before: beforeStatus, after: channels[i].Status})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, row := range auditRows {
		recordChannelStatusTransition(row.channelID, row.before, row.after, ChannelStatusSourceManualTag, actorID, audit...)
	}
	if common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return nil
}

func EnableChannelByTagWithActor(tag string, actorID int, audit ...ChannelStatusAudit) error {
	return updateChannelsStatusByTag(tag, common.ChannelStatusEnabled, actorID, audit...)
}

func DisableChannelByTagWithActor(tag string, actorID int, audit ...ChannelStatusAudit) error {
	return updateChannelsStatusByTag(tag, common.ChannelStatusManuallyDisabled, actorID, audit...)
}
