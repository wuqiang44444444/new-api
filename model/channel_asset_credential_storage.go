package model

import (
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type ChannelAssetCredential struct {
	ChannelID       int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	AccessKeyID     string `json:"-" gorm:"type:text;not null"`
	SecretAccessKey string `json:"-" gorm:"type:text;not null"`
	CreatedTime     int64  `json:"-" gorm:"bigint"`
	UpdatedTime     int64  `json:"-" gorm:"bigint"`
}

func GetChannelAssetCredential(channelID int) (*ChannelAssetCredential, error) {
	return getChannelAssetCredential(DB, channelID)
}

func getChannelAssetCredential(tx *gorm.DB, channelID int) (*ChannelAssetCredential, error) {
	var credential ChannelAssetCredential
	result := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id = ?", channelID).
		Limit(1).
		Find(&credential)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &credential, nil
}
