package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A manual reporting label, independent of URL, supplier APIs and routing.
// The row also serializes month edits after the Channel itself is deleted.
type ProviderChannelBillingConfig struct {
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Name      string `json:"upstream_name" gorm:"type:varchar(255);not null"`
}

func providerBillingChannelKnown(ctx context.Context, id int) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	for _, table := range []any{&Channel{}, &ProviderChannelBillingConfig{}, &ProviderChannelBillingDiscount{}} {
		var count int64
		column := "channel_id"
		if _, ok := table.(*Channel); ok {
			column = "id"
		}
		if err := DB.WithContext(ctx).Model(table).Where(column+" = ?", id).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	var rows []int
	err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("channel_id = ? AND type IN ?", id, []int{LogTypeConsume, LogTypeRefund}).Limit(1).Pluck("channel_id", &rows).Error
	return len(rows) > 0, err
}

func lockProviderBillingChannel(tx *gorm.DB, id int) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProviderChannelBillingConfig{ChannelId: id}).Error; err != nil {
		return err
	}
	var config ProviderChannelBillingConfig
	return lockForUpdate(tx).Where("channel_id = ?", id).First(&config).Error
}

func SaveProviderChannelBillingName(ctx context.Context, id int, name string, actor int) error {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 255 {
		return errors.New("upstream name is too long")
	}
	known, err := providerBillingChannelKnown(ctx, id)
	if err != nil {
		return err
	}
	if !known {
		return errors.New("channel not found")
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockProviderBillingChannel(tx, id); err != nil {
			return err
		}
		var before ProviderChannelBillingConfig
		if err := tx.Where("channel_id = ?", id).First(&before).Error; err != nil {
			return err
		}
		if before.Name == name {
			return nil
		}
		after := ProviderChannelBillingConfig{ChannelId: id, Name: name}
		if err := tx.Model(&after).Where("channel_id = ?", id).Update("name", name).Error; err != nil {
			return err
		}
		return createProviderBillingAudit(tx, "channel_name", strconv.Itoa(id), "update", &before, &after, "admin update of upstream display name", actor)
	})
}

func providerBillingChannelNames(ctx context.Context) (map[int]string, error) {
	result := map[int]string{}
	var rows []ProviderChannelBillingConfig
	err := DB.WithContext(ctx).Order("channel_id").FindInBatches(&rows, 500, func(_ *gorm.DB, _ int) error {
		for _, row := range rows {
			result[row.ChannelId] = row.Name
		}
		return nil
	}).Error
	return result, err
}

type ProviderChannelBillingConfigView struct {
	ChannelId       int                                `json:"channel_id"`
	ChannelName     string                             `json:"channel_name"`
	UpstreamName    string                             `json:"upstream_name"`
	Deleted         bool                               `json:"deleted"`
	Discount        *ProviderBillingDiscountProjection `json:"discount"`
	ExpectedVersion int64                              `json:"expected_version"`
}

func GetProviderChannelBillingConfig(ctx context.Context, id int, period int64) (ProviderChannelBillingConfigView, error) {
	result := ProviderChannelBillingConfigView{ChannelId: id}
	known, err := providerBillingChannelKnown(ctx, id)
	if err != nil {
		return result, err
	}
	if !known {
		return result, errors.New("channel not found")
	}
	var channels []Channel
	if err := DB.WithContext(ctx).Select("id, name").Where("id = ?", id).Find(&channels).Error; err != nil {
		return result, err
	}
	result.Deleted = len(channels) == 0
	if len(channels) > 0 {
		result.ChannelName = channels[0].Name
	}
	var configs []ProviderChannelBillingConfig
	if err := DB.WithContext(ctx).Where("channel_id = ?", id).Find(&configs).Error; err != nil {
		return result, err
	}
	if len(configs) > 0 {
		result.UpstreamName = configs[0].Name
	}
	records, err := loadProviderChannelBillingDiscounts(ctx, period, []int{id})
	if err != nil {
		return result, err
	}
	if record, ok := records[id]; ok {
		result.ExpectedVersion = record.Version
		if record.PendingReason == "" {
			projected := providerChannelDiscountProjection(record)
			result.Discount = &projected
		}
	}
	return result, nil
}
