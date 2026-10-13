package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const localChannelRC44MigrationKey = "migration.local_channel_rc44_types_v1"

// migrateLocalChannelRC44Types moves the previous fork identities away from
// rc.44's native vLLM/SGLang slots. Only identity numbers change; provider
// protocol, adapter revision, task status, credentials and funds stay frozen.
func migrateLocalChannelRC44Types() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)})
		var marker Option
		err := lockForUpdate(tx).Where(&Option{Key: localChannelRC44MigrationKey}).First(&marker).Error
		if err == nil && marker.Value == "done" {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		steps := []struct{ from, to int }{{62, constant.ChannelTypeSeedanceLink}, {63, constant.ChannelTypeAsyncImage}, {64, constant.ChannelTypeMiniMaxLink}, {66, constant.ChannelTypeAzureBatch}}
		for _, step := range steps {
			if err := tx.Model(&Channel{}).Where("type = ?", step.from).Update("type", step.to).Error; err != nil {
				return fmt.Errorf("rc44 channel identity migration: %w", err)
			}

		}
		if tx.Migrator().HasTable(&Task{}) {
			for _, step := range steps {
				if err := tx.Model(&Task{}).Where("platform = ?", strconv.Itoa(step.from)).Update("platform", strconv.Itoa(step.to)).Error; err != nil {
					return err
				}
			}
		}
		if tx.Migrator().HasTable(&TaskCreateAttempt{}) {
			for _, step := range steps {
				var attempts []TaskCreateAttempt
				if err := tx.Select("id", "adapter_version").Where("adapter_version LIKE ?", strconv.Itoa(step.from)+":%").Find(&attempts).Error; err != nil {
					return err
				}
				for _, attempt := range attempts {
					version := strconv.Itoa(step.to) + strings.TrimPrefix(attempt.AdapterVersion, strconv.Itoa(step.from))
					if err := tx.Model(&TaskCreateAttempt{}).Where("id = ?", attempt.ID).Update("adapter_version", version).Error; err != nil {
						return err
					}
				}
			}
		}
		for _, column := range []struct {
			model any
			field string
		}{{&Task{}, "private_data"}, {&TaskCreateAttempt{}, "recovery_snapshot"}, {&TaskCreateIdempotency{}, "recovery_snapshot"}} {
			if !tx.Migrator().HasTable(column.model) {
				continue
			}
			var lastID int64
			for {
				var rows []struct {
					ID    int64
					Facts string
				}
				if err := tx.Model(column.model).Select("id", column.field+" AS facts").Where("id > ?", lastID).Order("id").Limit(500).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) == 0 {
					break
				}
				for _, row := range rows {
					lastID = row.ID
					if strings.TrimSpace(row.Facts) == "" || row.Facts == "null" {
						continue
					}
					encoded, changed, err := remapRC44TaskIdentityFacts([]byte(row.Facts), steps)
					if err != nil {
						return fmt.Errorf("rc44 frozen identity migration: %s row %d contains invalid facts", column.field, row.ID)
					}
					if changed {
						if err := tx.Model(column.model).Where("id = ?", row.ID).Update(column.field, json.RawMessage(encoded)).Error; err != nil {
							return err
						}
					}
				}
			}
		}

		return tx.Save(&Option{Key: localChannelRC44MigrationKey, Value: "done"}).Error
	})
}

func verifyLocalChannelRC44Types() error {
	var marker Option
	if err := DB.Where(&Option{Key: localChannelRC44MigrationKey}).First(&marker).Error; err != nil || marker.Value != "done" {
		return errors.New("rc44 local channel identity migration is not complete")
	}
	return nil
}

// Decode only registered identity fields. RawMessage preserves unknown facts and funds.
func remapRC44TaskIdentityFacts(raw []byte, steps []struct{ from, to int }) ([]byte, bool, error) {
	var facts map[string]json.RawMessage
	if err := common.Unmarshal(raw, &facts); err != nil {
		return nil, false, err
	}
	if facts == nil {
		return raw, false, nil
	}
	changed := false
	if raw, ok := facts["southbound_adapter_version"]; ok {
		var version string
		if err := common.Unmarshal(raw, &version); err != nil {
			return nil, false, err
		}
		for _, step := range steps {
			if strings.HasPrefix(version, strconv.Itoa(step.from)+":") {
				facts["southbound_adapter_version"], _ = common.Marshal(strconv.Itoa(step.to) + strings.TrimPrefix(version, strconv.Itoa(step.from)))
				changed = true
				break
			}
		}
	}
	if raw, ok := facts["image_task"]; ok && string(raw) != "null" {
		var image map[string]json.RawMessage
		if err := common.Unmarshal(raw, &image); err != nil {
			return nil, false, err
		}
		if rawType, ok := image["channel_type"]; ok {
			var channelType int
			if err := common.Unmarshal(rawType, &channelType); err != nil {
				return nil, false, err
			}
			for _, step := range steps {
				if channelType == step.from {
					image["channel_type"], _ = common.Marshal(step.to)
					facts["image_task"], _ = common.Marshal(image)
					changed = true
					break
				}
			}
		}
	}
	// Recovery journals contain the same private task facts and a public Task envelope.
	if raw, ok := facts["private_data"]; ok && string(raw) != "null" {
		updated, remapped, err := remapRC44TaskIdentityFacts(raw, steps)
		if err != nil {
			return nil, false, err
		}
		if remapped {
			facts["private_data"] = updated
			changed = true
		}
	}
	if raw, ok := facts["task"]; ok && string(raw) != "null" {
		var task map[string]json.RawMessage
		if err := common.Unmarshal(raw, &task); err != nil {
			return nil, false, err
		}
		if rawPlatform, ok := task["platform"]; ok {
			var platform string
			if err := common.Unmarshal(rawPlatform, &platform); err != nil {
				return nil, false, err
			}
			for _, step := range steps {
				if platform == strconv.Itoa(step.from) {
					task["platform"], _ = common.Marshal(strconv.Itoa(step.to))
					facts["task"], _ = common.Marshal(task)
					changed = true
					break
				}
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	encoded, err := common.Marshal(facts)
	return encoded, true, err
}
