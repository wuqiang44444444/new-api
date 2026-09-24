package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// validateTypedStandardVideoModelConflict shares the management-only model
// ownership check across typed channels after their own configuration gates.
func validateTypedStandardVideoModelConflict(tx *gorm.DB, channel *Channel, label string) error {
	if tx == nil {
		tx = DB
	}
	models := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range channel.GetModels() {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		models = append(models, value)
	}
	if len(models) == 0 {
		return fmt.Errorf("%s channel requires at least one customer model", label)
	}
	var channels []Channel
	query := tx.Where("type IN ? AND status = ?", typedStandardVideoChannelTypes, common.ChannelStatusEnabled)
	if channel.Id > 0 {
		query = query.Where("id <> ?", channel.Id)
	}
	if err := query.Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		for _, modelName := range models {
			if channelContainsModel(&channels[i], modelName) {
				return fmt.Errorf("standard video model %q is already enabled on channel %q (#%d). Disable it there before enabling this channel", modelName, channels[i].Name, channels[i].Id)
			}
		}
	}
	return nil
}
