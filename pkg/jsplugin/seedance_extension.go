package jsplugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// MetaKeyFromSource compiles the module and returns only its declared meta
// key. Control planes use it to route a source to the right compile contract
// (generic task plugin vs Seedance extension) before full validation.
func MetaKeyFromSource(source string, options Options) (string, error) {
	engine, err := Compile(source, options)
	if err != nil {
		return "", err
	}
	value, err := engine.Export(context.Background(), "meta")
	if err != nil {
		return "", err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("plugin meta must be an object")
	}
	key, err := stringMetaField(object, "key")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(key), nil
}

// SeedanceExtensionProtocol names one code-registered Seedance upstream
// protocol together with the hooks a plugin must export to implement it.
type SeedanceExtensionProtocol struct {
	Name  string
	Hooks []string
}

// SeedanceExtensionContract is the host-owned definition of the Seedance link
// extension plugin kind. It is deliberately separate from the generic task
// plugin contract: a Seedance extension declares no models, channel types,
// routes, usage schema, or auth, so a compiled extension can never enter the
// native routing generation (by-model index, channel-type bindings, endpoint
// candidates, or model-matching eligibility).
type SeedanceExtensionContract struct {
	Key       string
	Protocols []SeedanceExtensionProtocol
	// AssetProtocols lists implemented host operations, not Provider pairing rules.
	AssetProtocols []string
}

// SeedanceExtensionInfo reports the protocols a compiled extension declares.
type SeedanceExtensionInfo struct {
	Protocols     []string
	Configuration *SeedanceChannelConfiguration
}

// CompileSeedanceExtension compiles and validates a Seedance link extension
// artifact. It shares the engine and the strict meta-decoding discipline with
// CompilePlugin, but validates Seedance protocol hooks instead of the generic
// task hooks and never registers the result in any Registry.
func CompileSeedanceExtension(source string, options Options, contract SeedanceExtensionContract) (*LoadedPlugin, SeedanceExtensionInfo, error) {
	if err := validateSeedanceExtensionContract(&contract); err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	engine, err := Compile(source, options)
	if err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	value, err := engine.Export(context.Background(), "meta")
	if err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	meta, protocols, err := decodeSeedanceExtensionMeta(value, contract)
	if err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	engine.key = meta.Key
	engine.version = meta.Version
	if err = validateSeedanceExtensionHooks(engine, contract, protocols); err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	configuration, err := decodeSeedanceChannelConfiguration(value, meta.APIVersion, protocols, contract.AssetProtocols)
	if err != nil {
		return nil, SeedanceExtensionInfo{}, err
	}
	if configuration != nil {
		if err = validateSeedanceAssetHooks(engine, configuration); err != nil {
			return nil, SeedanceExtensionInfo{}, err
		}
	}
	return &LoadedPlugin{Meta: meta, Engine: engine}, SeedanceExtensionInfo{Protocols: protocols, Configuration: configuration}, nil
}

func validateSeedanceExtensionContract(contract *SeedanceExtensionContract) error {
	if !pluginKeyPattern.MatchString(contract.Key) || len(contract.Key) > 30 {
		return fmt.Errorf("seedance extension contract key %q is invalid", contract.Key)
	}
	if len(contract.Protocols) == 0 {
		return fmt.Errorf("seedance extension contract declares no protocols")
	}
	seen := make(map[string]struct{}, len(contract.Protocols))
	for _, protocol := range contract.Protocols {
		if strings.TrimSpace(protocol.Name) == "" {
			return fmt.Errorf("seedance extension contract has an empty protocol name")
		}
		if _, duplicate := seen[protocol.Name]; duplicate {
			return fmt.Errorf("seedance extension contract repeats protocol %q", protocol.Name)
		}
		seen[protocol.Name] = struct{}{}
		if len(protocol.Hooks) == 0 {
			return fmt.Errorf("seedance extension protocol %q declares no hooks", protocol.Name)
		}
	}
	return nil
}

// decodeSeedanceExtensionMeta mirrors decodeMeta's strict decoding for the
// Seedance extension field set. Unknown fields — including every native
// routing field such as models, channelTypes, routes, protocols,
// usageSchema, fetchMode, allowedHosts, and auth — are rejected.
func decodeSeedanceExtensionMeta(value any, contract SeedanceExtensionContract) (Meta, []string, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return Meta{}, nil, fmt.Errorf("plugin meta must be an object")
	}
	for field := range object {
		switch field {
		case "apiVersion", "key", "name", "icon", "description", "version", "author", "seedanceProtocols", "channelConfiguration":
		default:
			return Meta{}, nil, fmt.Errorf("plugin meta has unknown field %q", field)
		}
	}
	meta := Meta{}
	var err error
	if meta.APIVersion, err = integerMetaField(object, "apiVersion"); err != nil {
		return Meta{}, nil, err
	}
	if meta.Key, err = stringMetaField(object, "key"); err != nil {
		return Meta{}, nil, err
	}
	if meta.Name, err = stringMetaField(object, "name"); err != nil {
		return Meta{}, nil, err
	}
	if meta.Icon, err = stringMetaField(object, "icon"); err != nil {
		return Meta{}, nil, err
	}
	meta.Icon = strings.TrimSpace(meta.Icon)
	if meta.Description, err = localizedTextMetaField(object, "description", maxMetaDescriptionRunes); err != nil {
		return Meta{}, nil, err
	}
	if meta.Version, err = stringMetaField(object, "version"); err != nil {
		return Meta{}, nil, err
	}
	author, ok := object["author"].(map[string]any)
	if !ok {
		return Meta{}, nil, fmt.Errorf("plugin meta author must be an object")
	}
	for field := range author {
		if field != "name" && field != "url" {
			return Meta{}, nil, fmt.Errorf("plugin meta author has unknown field %q", field)
		}
	}
	if meta.Author.Name, err = stringMetaField(author, "name"); err != nil {
		return Meta{}, nil, err
	}
	if rawURL, exists := author["url"]; exists {
		meta.Author.URL, ok = rawURL.(string)
		if !ok {
			return Meta{}, nil, fmt.Errorf("plugin meta author field %q must be a string", "url")
		}
	}
	protocols, err := strictStringSlice(object, "seedanceProtocols")
	if err != nil {
		return Meta{}, nil, err
	}
	if meta.APIVersion != APIVersion1 && meta.APIVersion != SeedanceConfigurationAPIVersion && meta.APIVersion != SeedanceUsageScanAPIVersion {
		return Meta{}, nil, fmt.Errorf("unsupported plugin apiVersion %d", meta.APIVersion)
	}
	if strings.TrimSpace(meta.Key) == "" || strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Version) == "" {
		return Meta{}, nil, fmt.Errorf("plugin meta key, name, and version are required")
	}
	if len(meta.Key) > 30 {
		return Meta{}, nil, fmt.Errorf("plugin meta key must not exceed 30 characters")
	}
	if !pluginKeyPattern.MatchString(meta.Key) {
		return Meta{}, nil, fmt.Errorf("plugin meta key %q is invalid", meta.Key)
	}
	if meta.Key != contract.Key {
		return Meta{}, nil, fmt.Errorf("seedance extension meta key must be %q", contract.Key)
	}
	if !pluginVersionPattern.MatchString(meta.Version) {
		return Meta{}, nil, fmt.Errorf("plugin meta version %q is not a semantic version", meta.Version)
	}
	if len(protocols) == 0 {
		return Meta{}, nil, fmt.Errorf("seedance extension must declare at least one protocol")
	}
	registered := make(map[string]struct{}, len(contract.Protocols))
	for _, protocol := range contract.Protocols {
		registered[protocol.Name] = struct{}{}
	}
	declared := make(map[string]struct{}, len(protocols))
	for _, protocol := range protocols {
		if _, duplicate := declared[protocol]; duplicate {
			return Meta{}, nil, fmt.Errorf("seedance extension declares protocol %q twice", protocol)
		}
		declared[protocol] = struct{}{}
		if _, known := registered[protocol]; !known {
			return Meta{}, nil, fmt.Errorf("seedance extension declares unregistered protocol %q", protocol)
		}
	}
	return meta, protocols, nil
}

func validateSeedanceExtensionHooks(engine *Engine, contract SeedanceExtensionContract, protocols []string) error {
	hooksByProtocol := make(map[string][]string, len(contract.Protocols))
	for _, protocol := range contract.Protocols {
		hooksByProtocol[protocol.Name] = protocol.Hooks
	}
	declared := make(map[string]struct{}, len(protocols))
	for _, protocol := range protocols {
		declared[protocol] = struct{}{}
	}
	rootValue, err := engine.Export(context.Background(), "seedance")
	if err != nil {
		return err
	}
	root, ok := rootValue.(map[string]any)
	if !ok {
		return fmt.Errorf("plugin %s export seedance must be an object", contract.Key)
	}
	for name := range root {
		if _, known := declared[name]; !known {
			return fmt.Errorf("plugin %s implements undeclared seedance protocol %q", contract.Key, name)
		}
	}
	names := append([]string(nil), protocols...)
	sort.Strings(names)
	for _, protocol := range names {
		allowed := make(map[string]struct{})
		for _, hook := range hooksByProtocol[protocol] {
			allowed[hook] = struct{}{}
			has, hasErr := engine.HasCallablePath(context.Background(), "seedance", protocol, hook)
			if hasErr != nil {
				return hasErr
			}
			if !has {
				return fmt.Errorf("plugin %s seedance protocol %q is missing required export seedance.%s.%s", contract.Key, protocol, protocol, hook)
			}
		}
		implementation, ok := root[protocol].(map[string]any)
		if !ok {
			return fmt.Errorf("plugin %s seedance protocol %q must be an object", contract.Key, protocol)
		}
		for member := range implementation {
			if _, accepted := allowed[member]; !accepted {
				return fmt.Errorf("plugin %s seedance protocol %q has unsupported member %q", contract.Key, protocol, member)
			}
		}
	}
	return nil
}

// Remote assets use bounded operations in the same artifact and engine as video.
// Hosted and none have no southbound JavaScript operations.
func validateSeedanceAssetHooks(engine *Engine, configuration *SeedanceChannelConfiguration) error {
	declared := make(map[string]bool)
	for _, asset := range configuration.Assets {
		if asset.Protocol != "none" && asset.GroupPolicy != "hosted" {
			declared[asset.Protocol] = true
		}
	}
	if len(declared) == 0 {
		return nil
	}
	value, err := engine.Export(context.Background(), "seedanceAssets")
	if err != nil {
		return err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("seedanceAssets must be an object")
	}
	for protocol := range root {
		if !declared[protocol] {
			return fmt.Errorf("undeclared asset implementation %q", protocol)
		}
	}
	for protocol := range declared {
		implementation, ok := root[protocol].(map[string]any)
		if !ok {
			return fmt.Errorf("missing asset implementation %q", protocol)
		}
		for _, hook := range []string{"buildRequest", "parseResponse"} {
			callable, err := engine.HasCallablePath(context.Background(), "seedanceAssets", protocol, hook)
			if err != nil {
				return err
			}
			if !callable {
				return fmt.Errorf("missing asset hook %s.%s", protocol, hook)
			}
		}
		for hook := range implementation {
			if hook != "buildRequest" && hook != "parseResponse" {
				return fmt.Errorf("unsupported asset hook %s.%s", protocol, hook)
			}
		}
	}
	return nil
}
