package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"gorm.io/gorm"
)

// MinimaxPluginConfiguration is the administrative declaration projection of
// the active minimax-link version. It does not return source code, Channel
// values or Provider credentials.
type MinimaxPluginConfiguration struct {
	Version       string                                 `json:"version"`
	Configuration *jsplugin.SeedanceChannelConfiguration `json:"configuration"`
}

// GetMinimaxPluginConfiguration reads the active version declaration.
func GetMinimaxPluginConfiguration() (*MinimaxPluginConfiguration, error) {
	active, err := GetTaskPluginVersion(jsplugin.MinimaxPluginKey, "")
	if err != nil {
		return nil, err
	}
	configuration, err := minimaxConfigurationForArtifact(active)
	if err != nil {
		return nil, err
	}
	return &MinimaxPluginConfiguration{Version: active.Version, Configuration: configuration}, nil
}

// PinMinimaxChannelConfigurationInput checks the form's declared version and
// carries it into the Channel write transaction without adding a database
// column. Bulk and status operations validate persisted values under the lock.
func PinMinimaxChannelConfigurationInput(channel *Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeMiniMaxLink {
		return nil
	}
	active, err := GetTaskPluginVersion(jsplugin.MinimaxPluginKey, "")
	if err != nil {
		return errors.New("MiniMax plugin configuration is unavailable")
	}
	if channel.MinimaxPluginVersion != "" && channel.MinimaxPluginVersion != active.Version {
		return errors.New("MiniMax plugin configuration changed; reload the channel form")
	}
	channel.MinimaxPluginVersion = active.Version
	return nil
}

// lockMinimaxPluginConfiguration serializes minimax-link publication against
// Channel configuration writes. PostgreSQL has no gap lock when the key has
// no versions yet, so a transaction advisory lock covers that window; MySQL's
// indexed UPDATE locks the key range; SQLite reserves the writer.
func lockMinimaxPluginConfiguration(tx *gorm.DB, key string) error {
	if key != jsplugin.MinimaxPluginKey {
		return nil
	}
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, hashtext(?))", int32(0x4d696e69), key).Error; err != nil {
			return err
		}
	}
	return tx.Model(&TaskPlugin{}).Where(&TaskPlugin{Key: key}).
		UpdateColumn("source_hash", gorm.Expr("source_hash")).Error
}

func minimaxConfigurationForArtifact(plugin *TaskPlugin) (*jsplugin.SeedanceChannelConfiguration, error) {
	loaded, info, err := jsplugin.CompileSeedanceExtension(plugin.Source, jsplugin.Options{}, jsplugin.MinimaxHostContract())
	if err != nil {
		return nil, fmt.Errorf("MiniMax plugin configuration cannot be compiled: %s", jsplugin.SeedanceCompilationDiagnostic(err))
	}
	if loaded.Meta.Key != plugin.Key || loaded.Meta.Version != plugin.Version || loaded.Meta.APIVersion != plugin.APIVersion {
		return nil, errors.New("MiniMax plugin identity does not match its stored artifact")
	}
	return info.Configuration, nil
}

// validateMinimaxPluginConfigurationActivation verifies the target version's
// declaration against every existing MiniMax Link channel before the active
// row changes.
func validateMinimaxPluginConfigurationActivation(tx *gorm.DB, target *TaskPlugin) error {
	if target.Key != jsplugin.MinimaxPluginKey {
		return nil
	}
	configuration, err := minimaxConfigurationForArtifact(target)
	if err != nil {
		return err
	}
	var channels []Channel
	if err = tx.Where("type = ?", constant.ChannelTypeMiniMaxLink).Order("id").Find(&channels).Error; err != nil {
		return err
	}
	affected := make([]int, 0)
	for i := range channels {
		if validateMinimaxChannelConfiguration(tx, &channels[i], configuration) != nil {
			affected = append(affected, channels[i].Id)
		}
	}
	if len(affected) != 0 {
		return fmt.Errorf("MiniMax plugin configuration is incompatible with channels %v", affected)
	}
	return nil
}

// validateMinimaxChannelConfiguration validates one channel against the
// declaration: the registered protocol, the none asset pairing and the
// resolved provider models must all be declared by the pinned version.
func validateMinimaxChannelConfiguration(tx *gorm.DB, channel *Channel, configuration *jsplugin.SeedanceChannelConfiguration) error {
	settings, err := minimaxChannelConfigurationValues(channel)
	if err != nil {
		return err
	}
	providerModels := make([]string, 0)
	for _, customerModel := range channel.GetModels() {
		providerModel, err := mappedCustomerModel(channel, strings.TrimSpace(customerModel))
		if err != nil {
			return err
		}
		providerModels = append(providerModels, providerModel)
	}
	return configuration.ValidateChannel(string(settings.VideoUpstreamProtocol), "none", providerModels, "", "", 0)
}

// validateMinimaxPublishedChannelConfiguration revalidates a channel against
// the active declaration inside the same transaction as its write.
func validateMinimaxPublishedChannelConfiguration(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeMiniMaxLink {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	if err := lockMinimaxPluginConfiguration(tx, jsplugin.MinimaxPluginKey); err != nil {
		return err
	}
	var active TaskPlugin
	if err := lockForUpdate(tx).Where(&TaskPlugin{Key: jsplugin.MinimaxPluginKey, Active: true}).First(&active).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("MiniMax plugin configuration is unavailable")
		}
		return err
	}
	if channel.MinimaxPluginVersion != "" && channel.MinimaxPluginVersion != active.Version {
		return errors.New("MiniMax plugin configuration changed; reload the channel form")
	}
	configuration, err := minimaxConfigurationForArtifact(&active)
	if err != nil {
		return err
	}
	return validateMinimaxChannelConfiguration(tx, channel, configuration)
}

func setMinimaxPluginConfigurationEnabled(key string, enabled bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockMinimaxPluginConfiguration(tx, key); err != nil {
			return err
		}
		var active TaskPlugin
		if err := lockForUpdate(tx).Where(&TaskPlugin{Key: key, Active: true}).First(&active).Error; err != nil {
			return err
		}
		if enabled {
			if err := validateMinimaxPluginConfigurationActivation(tx, &active); err != nil {
				return err
			}
		}
		return tx.Model(&active).Update("enabled", enabled).Error
	})
}

// validateMinimaxPluginConfigurationDeletion verifies that the version that
// would become active after a deletion still satisfies every channel.
func validateMinimaxPluginConfigurationDeletion(tx *gorm.DB, deleted *TaskPlugin) error {
	if deleted.Key != jsplugin.MinimaxPluginKey || !deleted.Active {
		return nil
	}
	var promoted TaskPlugin
	err := lockForUpdate(tx).Where(&TaskPlugin{Key: deleted.Key}).Where("id <> ?", deleted.Id).
		Order("created_at DESC, id DESC").First(&promoted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return validateMinimaxPluginConfigurationActivation(tx, &promoted)
}

// minimaxChannelConfigurationValues reads the shared OtherSettings JSON
// shape; MiniMax Link channels store the same field names as Seedance.
func minimaxChannelConfigurationValues(channel *Channel) (seedanceChannelConfigurationInput, error) {
	return seedanceChannelConfigurationValues(channel)
}
