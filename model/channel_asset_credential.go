package model

import (
	"crypto/sha256"
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

type ChannelAssetCredential struct {
	ChannelID       int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	AccessKeyID     string `json:"-" gorm:"type:text;not null"`
	SecretAccessKey string `json:"-" gorm:"type:text;not null"`
	CreatedTime     int64  `json:"-" gorm:"bigint"`
	UpdatedTime     int64  `json:"-" gorm:"bigint"`
}

var ErrAssetCredentialProfileActive = errors.New("separate asset credential profile must be disabled before clearing its credential")

const VolcengineAssetActionRegion = "cn-beijing"
const VolcengineAssetActionBaseURL = "https://ark.cn-beijing.volcengineapi.com"

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

func GetChannelAssetCredentialStatus(channelID int, includeHint bool) (dto.ChannelAssetCredentialStatus, error) {
	credential, err := GetChannelAssetCredential(channelID)
	if err != nil || credential == nil {
		return dto.ChannelAssetCredentialStatus{}, err
	}
	status := dto.ChannelAssetCredentialStatus{Configured: true}
	if includeHint {
		status.AccessKeyIDHint = MaskAssetAccessKeyID(credential.AccessKeyID)
	}
	return status, nil
}

func MaskAssetAccessKeyID(value string) string {
	value = strings.TrimSpace(value)
	characters := []rune(value)
	if len(characters) <= 5 {
		return strings.Repeat("*", len(characters))
	}
	return string(characters[:2]) + "******" + string(characters[len(characters)-3:])
}

func NormalizeChannelAssetCredential(input *dto.ChannelAssetCredentialInput) (*ChannelAssetCredential, error) {
	if input == nil {
		return nil, nil
	}
	accessKeyID := strings.TrimSpace(input.AccessKeyID)
	secretAccessKey := strings.TrimSpace(input.SecretAccessKey)
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, errors.New("asset Access Key ID and Secret Access Key must both be provided")
	}
	if strings.Contains(accessKeyID, "|") {
		return nil, errors.New("asset Access Key ID must not contain the credential separator")
	}
	return &ChannelAssetCredential{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}, nil
}

func BytePlusAssetActionBaseURL(region string) string {
	return "https://ark." + strings.TrimSpace(region) + ".byteplusapi.com"
}

func AssetActionBaseURL(protocol dto.AssetUpstreamProtocol, region string) string {
	switch protocol {
	case dto.AssetUpstreamProtocolVolcengineAction:
		return VolcengineAssetActionBaseURL
	case dto.AssetUpstreamProtocolBytePlusAction:
		return BytePlusAssetActionBaseURL(region)
	default:
		return ""
	}
}

func AssetCredentialFingerprint(baseURL, _ string, protocol string, providerScope ...string) string {
	input := strings.TrimRight(baseURL, "/") + "\n" + protocol
	for _, value := range providerScope {
		if strings.TrimSpace(value) != "" {
			input += "\n" + strings.TrimSpace(value)
		}
	}
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:])
}

func ResolveAssetChannelCredential(channel *Channel) (string, string, error) {
	return resolveAssetChannelCredential(DB, channel, nil)
}

func resolveAssetChannelCredential(tx *gorm.DB, channel *Channel, override *ChannelAssetCredential) (string, string, error) {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink || channel.ChannelInfo.IsMultiKey {
		return "", "", errors.New("asset channel must use a single credential")
	}
	settings := channel.GetOtherSettings()
	assetProfile := settings.AssetUpstreamProtocol.TransportProfile()
	credentialIdentity := string(settings.AssetUpstreamProtocol)
	if assetProfile == dto.AssetUpstreamProfileOfficial || assetProfile == dto.AssetUpstreamProfileCMCCAICCV2 {
		credential := override
		var err error
		if credential == nil {
			credential, err = getChannelAssetCredential(tx, channel.Id)
			if err != nil {
				return "", "", err
			}
		}
		if credential == nil || strings.TrimSpace(credential.AccessKeyID) == "" || strings.TrimSpace(credential.SecretAccessKey) == "" {
			return "", "", errors.New("separate asset credential is not configured")
		}
		key := strings.TrimSpace(credential.AccessKeyID) + "|" + strings.TrimSpace(credential.SecretAccessKey)
		if assetProfile == dto.AssetUpstreamProfileCMCCAICCV2 {
			scope, err := cmccAssetReuseScope(tx, channel.Id)
			return key, strings.TrimPrefix(scope, "asset_scope_"), err
		}
		return key, AssetCredentialFingerprint(
			AssetActionBaseURL(settings.AssetUpstreamProtocol, settings.AssetRegion),
			key,
			credentialIdentity,
			settings.AssetProviderProject,
			settings.AssetRegion,
		), nil
	}
	keys := channel.GetKeys()
	if len(keys) != 1 || strings.TrimSpace(keys[0]) == "" {
		return "", "", errors.New("asset channel must contain exactly one credential")
	}
	key := strings.TrimSpace(keys[0])
	return key, AssetCredentialFingerprint(
		channel.GetBaseURL(),
		key,
		credentialIdentity,
		settings.AssetProviderProject,
		settings.AssetRegion,
	), nil
}

func InsertChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
	return InsertChannelWithAssetCredentialActor(channel, input, 0)
}

func InsertChannelWithAssetCredentialActor(channel *Channel, input *dto.ChannelAssetCredentialInput, actorID int) error {
	credential, err := NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if channel == nil || credential == nil {
		return errors.New("channel and asset credential are required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		if err := channel.AddAbilitiesWithActor(tx, actorID); err != nil {
			return err
		}
		now := common.GetTimestamp()
		credential.ChannelID = channel.Id
		credential.CreatedTime = now
		credential.UpdatedTime = now
		return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Create(credential).Error
	})
}

func UpdateChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
	return UpdateChannelWithAssetCredentialActor(channel, input, 0)
}

func UpdateChannelWithAssetCredentialActor(channel *Channel, input *dto.ChannelAssetCredentialInput, actorID int) error {
	credential, err := NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if channel == nil || credential == nil {
		return errors.New("channel and asset credential are required")
	}
	credential.ChannelID = channel.Id
	return updateChannelWithCredentialActor(channel, credential, actorID)
}

func DeleteChannelAssetCredential(channelID int) error {
	if channelID <= 0 {
		return errors.New("channel ID is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		channel, err := lockChannelForMutation(tx, channelID)
		if err != nil {
			return err
		}
		settings := channel.GetOtherSettings()
		assetProfile := settings.AssetUpstreamProtocol.TransportProfile()
		if assetProfile == dto.AssetUpstreamProfileOfficial || assetProfile == dto.AssetUpstreamProfileCMCCAICCV2 {
			return ErrAssetCredentialProfileActive
		}
		return deleteChannelAssetCredentialsTx(tx, []int{channelID})
	})
}

func saveChannelAssetCredentialTx(tx *gorm.DB, credential *ChannelAssetCredential) error {
	now := common.GetTimestamp()
	credential.UpdatedTime = now
	if credential.CreatedTime == 0 {
		credential.CreatedTime = now
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"access_key_id":     credential.AccessKeyID,
			"secret_access_key": credential.SecretAccessKey,
			"updated_time":      credential.UpdatedTime,
		}),
	}).Create(credential).Error
}

func deleteChannelAssetCredentialsTx(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelAssetCredential{}) {
		return nil
	}
	if err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id IN ?", channelIDs).
		Delete(&ChannelAssetCredential{}).Error; err != nil {
		return fmt.Errorf("delete channel asset credentials: %w", err)
	}
	return nil
}
