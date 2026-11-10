package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

func lockChannelForMutation(tx *gorm.DB, id int) (*Channel, error) {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := tx.Model(&Channel{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return nil, err
		}
	}
	var channel Channel
	if err := lockForUpdate(tx).First(&channel, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// UpdateMiniMaxProtocolAsDisabled applies the channel edit and manual stop
// together, so neither the old nor the new protocol is exposed mid-update.
func (channel *Channel) UpdateMiniMaxProtocolAsDisabled(actorID int, audit ChannelStatusAudit) error {
	return updateChannelWithCredentialActor(channel, nil, actorID, false, false, true, audit)
}

func updateChannelWithCredentialActor(
	channel *Channel,
	credential *ChannelAssetCredential,
	actorID int,
	assetTenantUnchanged bool,
	assetTenantReplacementConfirmed bool,
	saveMiniMaxAsDisabled bool,
	audit ...ChannelStatusAudit,
) error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	var beforeStatus, afterStatus int
	err := DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockChannelForMutation(tx, channel.Id)
		if err != nil {
			return err
		}
		original := *current
		beforeStatus = original.Status
		if err := tx.Model(current).Updates(channel).Error; err != nil {
			return err
		}
		if err := tx.First(channel, "id = ?", channel.Id).Error; err != nil {
			return err
		}
		originalSettings, err := parsedChannelOtherSettings(&original)
		if err != nil {
			return err
		}
		miniMaxProtocolChanged := false
		if original.Type == constant.ChannelTypeMiniMaxLink && channel.Type == original.Type {
			settings, err := parsedChannelOtherSettings(channel)
			if err != nil {
				return err
			}
			miniMaxProtocolChanged = originalSettings.VideoUpstreamProtocol != settings.VideoUpstreamProtocol
			if miniMaxProtocolChanged {
				savedSettings := channel.OtherSettings
				if err := validateMinimaxChannelSettingsTx(tx, channel, &settings); err != nil {
					return err
				}
				if channel.OtherSettings != savedSettings {
					if err := tx.Model(channel).UpdateColumn("settings", channel.OtherSettings).Error; err != nil {
						return err
					}
				}
			}
		}
		if saveMiniMaxAsDisabled {
			if !miniMaxProtocolChanged {
				return common.NewMessage("Change the MiniMax Link access method to save as disabled.")
			}
			if channel.Status != common.ChannelStatusManuallyDisabled {
				info := channel.GetOtherInfo()
				info["status_reason"] = "manual operation"
				info["status_time"] = common.GetTimestamp()
				channel.SetOtherInfo(info)
				channel.Status = common.ChannelStatusManuallyDisabled
				if err := tx.Model(channel).Select("status", "other_info").Updates(channel).Error; err != nil {
					return err
				}
			}
		} else if miniMaxProtocolChanged && channel.Status == common.ChannelStatusEnabled {
			if err := validateMiniMaxChannelPricing(channel); err != nil {
				return err
			}
		}
		if err := validateChannelAssetTenantMutation(
			tx,
			&original,
			channel,
			credential,
			assetTenantUnchanged,
			assetTenantReplacementConfirmed,
		); err != nil {
			return err
		}
		boundaryChanges, err := ChannelAssetTenantBoundaryChanges(&original, channel)
		if err != nil {
			return err
		}
		boundaryEstablished, err := channelAssetTenantBoundaryEstablished(tx, &original, originalSettings)
		if err != nil {
			return err
		}
		if boundaryEstablished && len(boundaryChanges) > 0 && assetTenantReplacementConfirmed {
			if err := deleteChannelDefaultAssetGroupsTx(tx, []int{channel.Id}); err != nil {
				return err
			}
			if err := replaceChannelAssetScopeIdentityTx(tx, channel); err != nil {
				return err
			}
		}
		if credential != nil {
			if err := saveChannelAssetCredentialTx(tx, credential); err != nil {
				return err
			}
		}
		afterStatus = channel.Status
		return channel.UpdateAbilitiesWithActor(tx, actorID)
	})
	if err == nil {
		recordChannelStatusTransition(channel.Id, beforeStatus, afterStatus, ChannelStatusSourceManual, actorID, audit...)
	}
	return err
}

func deleteChannel(channel *Channel) error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	rows, err := batchDeleteChannelRows([]int{channel.Id})
	if err != nil {
		return err
	}
	if rows == 0 {
		return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	}
	return nil
}

func batchDeleteChannelRows(ids []int) (int64, error) {
	ids = sortedUniqueChannelIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	var rows int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		deletable := make([]int, 0, len(ids))
		for _, id := range ids {
			channel, err := lockChannelForMutation(tx, id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			deletable = append(deletable, channel.Id)
		}
		result := tx.Where("id IN ?", deletable).Delete(&Channel{})
		rows = result.RowsAffected
		if result.Error != nil {
			return result.Error
		}
		if err := tx.Where("channel_id IN ?", deletable).Delete(&Ability{}).Error; err != nil {
			return err
		}
		if err := deleteChannelAssetCredentialsTx(tx, deletable); err != nil {
			return err
		}
		if err := deleteChannelDefaultAssetGroupsTx(tx, deletable); err != nil {
			return err
		}
		return deleteChannelAssetScopeIdentitiesTx(tx, deletable)
	})
	return rows, err
}

func deleteChannelsByStatus(statuses []int) (int64, error) {
	var ids []int
	if err := DB.Model(&Channel{}).Where("status IN ?", statuses).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return batchDeleteChannelRows(ids)
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
