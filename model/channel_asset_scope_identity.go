package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

type ChannelAssetScopeIdentity struct {
	ChannelID   int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Identity    string `json:"-" gorm:"type:varchar(64);not null"`
	CreatedTime int64  `json:"-" gorm:"bigint"`
}

func ensureChannelAssetScopeIdentityTx(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Id <= 0 ||
		channel.GetOtherSettings().AssetUpstreamProtocol != dto.AssetUpstreamProtocolCMCCAICCV2 {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return fmt.Errorf("generate channel asset scope identity: %w", err)
	}
	record := ChannelAssetScopeIdentity{
		ChannelID: channel.Id, Identity: hex.EncodeToString(random), CreatedTime: common.GetTimestamp(),
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error
}

func getChannelAssetScopeIdentity(tx *gorm.DB, channelID int) (string, error) {
	if tx == nil || channelID <= 0 {
		return "", errors.New("channel asset scope identity is unavailable")
	}
	var record ChannelAssetScopeIdentity
	result := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id = ?", channelID).Limit(1).Find(&record)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 || strings.TrimSpace(record.Identity) == "" {
		return "", errors.New("channel asset scope identity is not configured")
	}
	return strings.TrimSpace(record.Identity), nil
}

func CMCCAssetReuseScope(channelID int) (string, error) {
	return cmccAssetReuseScope(DB, channelID)
}

func cmccAssetReuseScope(tx *gorm.DB, channelID int) (string, error) {
	identity, err := getChannelAssetScopeIdentity(tx, channelID)
	if err != nil {
		return "", err
	}
	input := string(dto.AssetUpstreamProtocolCMCCAICCV2) + "\n" +
		strings.TrimRight(CMCCAICCAssetBaseURL, "/") + "\n" + identity
	sum := sha256.Sum256([]byte(input))
	return "asset_scope_" + hex.EncodeToString(sum[:]), nil
}

func deleteChannelAssetScopeIdentitiesTx(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelAssetScopeIdentity{}) {
		return nil
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id IN ?", channelIDs).Delete(&ChannelAssetScopeIdentity{}).Error
}
