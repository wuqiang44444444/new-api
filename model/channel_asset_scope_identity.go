package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

const channelAssetScopeFormatVersion = "seedance_channel_asset_scope:v1"

var errChannelAssetScopeIdentityMissing = errors.New("channel asset scope identity is not configured")

type ChannelAssetScopeIdentity struct {
	ChannelID   int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Identity    string `json:"-" gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedTime int64  `json:"-" gorm:"bigint"`
}

func ensureChannelAssetScopeIdentityTx(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Id <= 0 || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	settings, err := parsedChannelOtherSettings(channel)
	if err != nil {
		return err
	}
	if settings.AssetUpstreamProtocol == "" || settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone {
		return nil
	}
	if tx == nil {
		tx = DB
	}

	quiet := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)})
	identity, err := newChannelAssetScopeIdentity()
	if err != nil {
		return fmt.Errorf("generate channel asset scope identity: %w", err)
	}
	record := ChannelAssetScopeIdentity{
		ChannelID: channel.Id, Identity: identity, CreatedTime: common.GetTimestamp(),
	}
	if err := quiet.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoNothing: true,
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("create channel asset scope identity: %w", err)
	}
	if _, err := getChannelAssetScopeIdentity(quiet, channel.Id); err != nil {
		return fmt.Errorf("verify channel asset scope identity: %w", err)
	}
	return nil
}

func replaceChannelAssetScopeIdentityTx(tx *gorm.DB, channel *Channel) error {
	if tx == nil || channel == nil || channel.Id <= 0 {
		return errors.New("channel asset scope identity is unavailable")
	}
	settings, err := parsedChannelOtherSettings(channel)
	if err != nil {
		return err
	}
	if channel.Type != constant.ChannelTypeSeedanceLink ||
		settings.AssetUpstreamProtocol == "" || settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone {
		return deleteChannelAssetScopeIdentitiesTx(tx, []int{channel.Id})
	}
	identity, err := newChannelAssetScopeIdentity()
	if err != nil {
		return fmt.Errorf("generate channel asset scope identity: %w", err)
	}
	record := ChannelAssetScopeIdentity{
		ChannelID: channel.Id, Identity: identity, CreatedTime: common.GetTimestamp(),
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"identity":     record.Identity,
			"created_time": record.CreatedTime,
		}),
	}).Create(&record).Error
}

func newChannelAssetScopeIdentity() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
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
		return "", errChannelAssetScopeIdentityMissing
	}
	return strings.TrimSpace(record.Identity), nil
}

func ChannelAssetReuseScope(channelID int) (string, error) {
	identity, err := getChannelAssetScopeIdentity(DB, channelID)
	if err != nil {
		return "", err
	}
	return channelAssetReuseScope(identity), nil
}

func channelAssetReuseScope(identity string) string {
	input := channelAssetScopeFormatVersion + "\n" + strings.TrimSpace(identity)
	sum := sha256.Sum256([]byte(input))
	return "asset_scope_" + hex.EncodeToString(sum[:])
}

func loadChannelAssetReuseScopes(tx *gorm.DB, channelIDs []int) (map[int]string, error) {
	result := make(map[int]string, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}
	var records []ChannelAssetScopeIdentity
	if err := tx.Where("channel_id IN ?", channelIDs).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		if identity := strings.TrimSpace(record.Identity); identity != "" {
			result[record.ChannelID] = channelAssetReuseScope(identity)
		}
	}
	return result, nil
}

func backfillChannelAssetScopeIdentities() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Where("type = ?", constant.ChannelTypeSeedanceLink).Find(&channels).Error; err != nil {
			return err
		}
		for i := range channels {
			if err := ensureChannelAssetScopeIdentityTx(tx, &channels[i]); err != nil {
				return fmt.Errorf("backfill asset scope identity for channel %d: %w", channels[i].Id, err)
			}
		}
		return nil
	})
}

func deleteChannelAssetScopeIdentitiesTx(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelAssetScopeIdentity{}) {
		return nil
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id IN ?", channelIDs).Delete(&ChannelAssetScopeIdentity{}).Error
}
