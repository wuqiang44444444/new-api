package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
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

func updateChannelWithCredentialActor(channel *Channel, credential *ChannelAssetCredential, actorID int) error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockChannelForMutation(tx, channel.Id)
		if err != nil {
			return err
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
		return deleteChannelAssetCredentialsTx(tx, deletable)
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
