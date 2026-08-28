package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

type ChannelDefaultAssetGroup struct {
	ChannelID       int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	ProviderGroupID string `json:"-" gorm:"type:text;not null"`
}

func GetChannelDefaultAssetGroup(channelID int) (*ChannelDefaultAssetGroup, error) {
	if channelID <= 0 {
		return nil, errors.New("channel ID is invalid")
	}
	var record ChannelDefaultAssetGroup
	result := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id = ?", channelID).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func SaveChannelDefaultAssetGroup(channelID int, providerGroupID string) error {
	if channelID <= 0 || strings.TrimSpace(providerGroupID) == "" {
		return errors.New("channel default asset group is invalid")
	}
	record := ChannelDefaultAssetGroup{ChannelID: channelID, ProviderGroupID: providerGroupID}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"provider_group_id": providerGroupID,
		}),
	}).Create(&record).Error
}

func deleteChannelDefaultAssetGroupsTx(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelDefaultAssetGroup{}) {
		return nil
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id IN ?", channelIDs).Delete(&ChannelDefaultAssetGroup{}).Error
}
