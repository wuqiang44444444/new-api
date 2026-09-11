package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"gorm.io/gorm"
)

type SeedancePluginConfiguration struct {
	Version       string                                 `json:"version"`
	Configuration *jsplugin.SeedanceChannelConfiguration `json:"configuration"`
}

// GetSeedancePluginConfiguration is the administrative declaration projection.
// It does not return source code, Channel values or Provider credentials.
func GetSeedancePluginConfiguration() (*SeedancePluginConfiguration, error) {
	active, err := GetTaskPluginVersion(jsplugin.SeedancePluginKey, "")
	if err != nil {
		return nil, err
	}
	configuration, err := seedanceConfigurationForArtifact(active)
	if err != nil {
		return nil, err
	}
	return &SeedancePluginConfiguration{Version: active.Version, Configuration: configuration}, nil
}

// PinSeedanceChannelConfigurationInput checks the form's declared version and
// carries it into the Channel write transaction without adding a database column.
// A v1 form is pinned too, so promotion to v2 during its save cannot bypass the
// later check. Bulk/status operations validate persisted values under the lock.
func PinSeedanceChannelConfigurationInput(channel *Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	settings, err := seedanceChannelConfigurationValues(channel)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(jsplugin.SeedanceHostContract().Protocols, func(protocol jsplugin.SeedanceExtensionProtocol) bool {
		return protocol.Name == string(settings.VideoUpstreamProtocol)
	}) {
		return nil
	}
	active, err := GetTaskPluginVersion(jsplugin.SeedancePluginKey, "")
	if err != nil {
		return errors.New("Seedance plugin configuration is unavailable")
	}
	if (active.APIVersion >= jsplugin.SeedanceConfigurationAPIVersion || channel.SeedancePluginVersion != "") && channel.SeedancePluginVersion != active.Version {
		return errors.New("Seedance plugin configuration changed; reload the channel form")
	}
	channel.SeedancePluginVersion = active.Version
	return nil
}

// Lock configuration publication before inspecting an active version. The same
// transaction lock is used by Channel configuration writes. Activation reads
// Channels without acquiring their row locks, so a writer holding a Channel
// lock can wait here without creating a reverse lock order.
func lockSeedancePluginConfiguration(tx *gorm.DB, key string) error {
	if key != jsplugin.SeedancePluginKey {
		return nil
	}
	// PostgreSQL has no gap lock when the key has no versions yet. Use a
	// transaction advisory lock for that first-publication window as well.
	// MySQL's indexed UPDATE locks the key range; SQLite reserves the writer.
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, hashtext(?))", int32(0x53644e63), key).Error; err != nil {
			return err
		}
	}
	return tx.Model(&TaskPlugin{}).Where(&TaskPlugin{Key: key}).
		UpdateColumn("source_hash", gorm.Expr("source_hash")).Error
}

func seedanceConfigurationForArtifact(plugin *TaskPlugin) (*jsplugin.SeedanceChannelConfiguration, error) {
	loaded, info, err := jsplugin.CompileSeedanceExtension(plugin.Source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
	if err != nil {
		return nil, fmt.Errorf("Seedance plugin configuration cannot be compiled: %s", jsplugin.SeedanceCompilationDiagnostic(err))
	}
	if loaded.Meta.Key != plugin.Key || loaded.Meta.Version != plugin.Version || loaded.Meta.APIVersion != plugin.APIVersion {
		return nil, errors.New("Seedance plugin identity does not match its stored artifact")
	}
	return info.Configuration, nil
}

// Validate promotion before the existing publication transaction changes the
// active row. It lists only administrator-visible Channel IDs; no credentials,
// connections, model identities or Provider error bodies enter the error.
func validateSeedancePluginConfigurationActivation(tx *gorm.DB, target *TaskPlugin) error {
	if target.Key != jsplugin.SeedancePluginKey {
		return nil
	}
	var current TaskPlugin
	err := lockForUpdate(tx).Where(&TaskPlugin{Key: target.Key, Active: true}).First(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// v1 has no configuration contract. Its existing release and history paths
	// are unchanged until configuration publication is actually introduced.
	if target.APIVersion < jsplugin.SeedanceConfigurationAPIVersion && current.APIVersion < jsplugin.SeedanceConfigurationAPIVersion {
		return nil
	}
	configuration, err := seedanceConfigurationForArtifact(target)
	if err != nil {
		return err
	}
	var channels []Channel
	if err = tx.Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		return err
	}
	affected := make([]int, 0)
	for i := range channels {
		settings, err := seedanceChannelConfigurationValues(&channels[i])
		if err != nil {
			return err
		}
		migrated := false
		for _, protocol := range jsplugin.SeedanceHostContract().Protocols {
			if protocol.Name == string(settings.VideoUpstreamProtocol) {
				migrated = true
				break
			}
		}
		if !migrated {
			continue
		}
		if configuration == nil || validateSeedanceChannelConfiguration(tx, &channels[i], configuration) != nil {
			affected = append(affected, channels[i].Id)
		}
	}
	if len(affected) != 0 {
		return fmt.Errorf("Seedance plugin configuration is incompatible with channels %v", affected)
	}
	return nil
}

func validateSeedanceChannelConfiguration(tx *gorm.DB, channel *Channel, configuration *jsplugin.SeedanceChannelConfiguration) error {
	settings, err := seedanceChannelConfigurationValues(channel)
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
	assetProtocol := string(settings.AssetUpstreamProtocol)
	if assetProtocol == "" {
		assetProtocol = "none"
	}
	if err = configuration.ValidateChannel(string(settings.VideoUpstreamProtocol), assetProtocol, providerModels,
		settings.AssetProviderProject, settings.AssetRegion, settings.AssetMinURLTTLSeconds); err != nil {
		return err
	}
	for _, asset := range configuration.Assets {
		if asset.Protocol != assetProtocol || asset.Credential != "asset_key_pair" {
			continue
		}
		credential, err := getChannelAssetCredential(tx, channel.Id)
		if err != nil {
			return err
		}
		if credential == nil || strings.TrimSpace(credential.AccessKeyID) == "" || strings.TrimSpace(credential.SecretAccessKey) == "" {
			return errors.New("separate asset credential is not configured")
		}
	}
	return nil
}

// Revalidate a Channel in the same write transaction as its persisted values.
// The declaration has no default-write behavior. v1 channels retain their
// existing rules; only a published v2 definition becomes configuration owner.
func validateSeedancePublishedChannelConfiguration(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	settings, err := seedanceChannelConfigurationValues(channel)
	if err != nil {
		return err
	}
	protocols := jsplugin.SeedanceHostContract().Protocols
	if !slices.ContainsFunc(protocols, func(protocol jsplugin.SeedanceExtensionProtocol) bool {
		return protocol.Name == string(settings.VideoUpstreamProtocol)
	}) {
		return nil
	}
	if err := lockSeedancePluginConfiguration(tx, jsplugin.SeedancePluginKey); err != nil {
		return err
	}
	var active TaskPlugin
	if err := lockForUpdate(tx).Where(&TaskPlugin{Key: jsplugin.SeedancePluginKey, Active: true}).First(&active).Error; err != nil {
		return err
	}
	if channel.SeedancePluginVersion != "" && channel.SeedancePluginVersion != active.Version {
		return errors.New("Seedance plugin configuration changed; reload the channel form")
	}
	if active.APIVersion < jsplugin.SeedanceConfigurationAPIVersion {
		return nil
	}
	configuration, err := seedanceConfigurationForArtifact(&active)
	if err != nil {
		return err
	}
	return validateSeedanceChannelConfiguration(tx, channel, configuration)
}

func setSeedancePluginConfigurationEnabled(key string, enabled bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSeedancePluginConfiguration(tx, key); err != nil {
			return err
		}
		var active TaskPlugin
		if err := lockForUpdate(tx).Where(&TaskPlugin{Key: key, Active: true}).First(&active).Error; err != nil {
			return err
		}
		if enabled {
			if err := validateSeedancePluginConfigurationActivation(tx, &active); err != nil {
				return err
			}
		}
		return tx.Model(&active).Update("enabled", enabled).Error
	})
}

func validateSeedancePluginConfigurationDeletion(tx *gorm.DB, deleted *TaskPlugin) error {
	if deleted.Key != jsplugin.SeedancePluginKey || !deleted.Active {
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
	return validateSeedancePluginConfigurationActivation(tx, &promoted)
}

// seedanceChannelConfigurationInput is a read projection of existing Channel
// fields. It adds no persistence, defaults or second configuration source.
type seedanceChannelConfigurationInput struct {
	VideoUpstreamProtocol string `json:"video_upstream_protocol"`
	AssetUpstreamProtocol string `json:"asset_upstream_protocol"`
	AssetProviderProject  string `json:"asset_provider_project"`
	AssetRegion           string `json:"asset_region"`
	AssetMinURLTTLSeconds int64  `json:"asset_min_url_ttl_seconds"`
}

func seedanceChannelConfigurationValues(channel *Channel) (seedanceChannelConfigurationInput, error) {
	var values seedanceChannelConfigurationInput
	if channel == nil || strings.TrimSpace(channel.OtherSettings) == "" {
		return values, nil
	}
	err := common.UnmarshalJsonStr(channel.OtherSettings, &values)
	return values, err
}
