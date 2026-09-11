package seedance

import (
	"context"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"time"

	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// SeedanceExtensionPluginKey is the reserved plugin key of the Seedance Link
// extension artifact. It never collides with the generic task-plugin key
// space: control-plane compilation routes this key to the Seedance extension
// contract, and the native registry never loads it.
const SeedanceExtensionPluginKey = pluginruntime.SeedancePluginKey

// seedanceExtensionProbeFields lists the billing-probe keys a plugin
// conversion may contribute per protocol. The whitelist is the host-owned
// billing boundary: host-derived facts (duration_seconds, has_video_input,
// generate_audio, input_mode, control_mode) never appear here, so plugin
// output can never rewrite the inputs that determine charged amounts.
var seedanceExtensionProbeFields = map[dto.VideoUpstreamProtocol]map[string]struct{}{
	dto.VideoUpstreamProtocolFeicaiVideosV1: {
		"resolution": {}, "ratio": {}, "size_multiplier": {}, "billing_mode": {},
	},
}

// seedanceExtensionCreateAdmissionTimeout bounds how long a create-path hook
// waits for an engine slot before the request is rejected. The rejection
// happens before any funds are held. Execution itself still gets the engine's
// full call timeout.
var seedanceExtensionCreateAdmissionTimeout = 2 * time.Second

// seedanceExtensionPollAdmissionTimeout bounds slot wait time for polling
// and recovery hooks. A timeout here is an observation failure, never an
// automatic refund or task failure.
var seedanceExtensionPollAdmissionTimeout = 2 * time.Second

// SeedanceExtensionContract returns the host-registered extension contract:
// the reserved key plus the per-protocol required hooks.
func SeedanceExtensionContract() pluginruntime.SeedanceExtensionContract {
	return pluginruntime.SeedanceHostContract()
}

// SeedanceExtensionProtocolMigrated reports whether new requests for this
// protocol are served by the extension plugin. The shared host contract is
// the only migration registry; historical tasks keep their frozen execution.
func SeedanceExtensionProtocolMigrated(protocol dto.VideoUpstreamProtocol) bool {
	for _, registered := range pluginruntime.SeedanceHostContract().Protocols {
		if registered.Name == string(protocol) {
			return true
		}
	}
	return false
}

// CompileSeedanceExtensionSource compiles an administrator-provided artifact
// against the host contract. It is the control-plane entry for upload,
// activation, listing, and dry-run compilation.
func CompileSeedanceExtensionSource(source string) (*pluginruntime.LoadedPlugin, pluginruntime.SeedanceExtensionInfo, error) {
	return pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{Key: SeedanceExtensionPluginKey}, SeedanceExtensionContract())
}

// EnsureSeededExtension seeds the embedded artifact into the version store
// (idempotent per process and per version).
func EnsureSeededExtension(ctx context.Context) error {
	return seedanceplugin.Default.EnsureSeeded(ctx)
}

// SyncExtensionSnapshot publishes the active database rows to the extension
// store.
func SyncExtensionSnapshot(ctx context.Context, rows []model.TaskPlugin) error {
	return seedanceplugin.Default.SyncSnapshot(ctx, rows)
}

// DescribeExtension returns control-plane diagnostics for the extension.
func DescribeExtension() (activeVersion string, syncErrors []string) {
	return seedanceplugin.Default.Describe()
}

// PinSeedanceExtensionForChannel resolves the active extension version for a
// channel whose video protocol has been migrated and pins it in the gin
// context so the submission snapshot freezes plugin identity before any
// funds are held. Channels on non-migrated protocols pin nothing. A missing
// or non-covering active version is an error: creation must fail closed.
// Hook presence is verified at compile time and protocol coverage by
// ActiveFor; the request path performs no engine calls, so there is no
// unbounded admission wait here.
func PinSeedanceExtensionForChannel(c *gin.Context, videoProtocol dto.VideoUpstreamProtocol) error {
	if !SeedanceExtensionProtocolMigrated(videoProtocol) {
		return nil
	}
	entry, err := seedanceplugin.Default.ActiveEntryFor(videoProtocol)
	if err != nil {
		return err
	}
	c.Set(seedanceConfigurationContextKey, entry)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: entry.Plugin})
	return nil
}

const seedanceConfigurationContextKey = "seedance_pinned_configuration"

// PinnedSeedanceConfiguration returns only the declaration paired with this
// request's code. Missing v2 declarations fail closed; historical v1 has none.
func PinnedSeedanceConfiguration(c *gin.Context) (*pluginruntime.SeedanceChannelConfiguration, error) {
	plugin := pinnedSeedanceExtension(c)
	if plugin == nil {
		return nil, seedanceplugin.ErrUnavailable
	}
	if plugin.Meta.APIVersion == pluginruntime.APIVersion1 {
		return nil, nil
	}
	value, exists := c.Get(seedanceConfigurationContextKey)
	entry, ok := value.(*seedanceplugin.CompiledVersion)
	if !exists || !ok || entry.Plugin != plugin || entry.Info.Configuration == nil {
		return nil, seedanceplugin.ErrUnavailable
	}
	return entry.Info.Configuration, nil
}
