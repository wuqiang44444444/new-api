package seedance

import (
	"context"
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
const SeedanceExtensionPluginKey = "seedance-link"

// seedanceExtensionProtocols is the single migration registry. A protocol
// listed here has its southbound conversion served by the seedance-link
// extension plugin for new requests; when the plugin or a required hook is
// unavailable, creation fails closed instead of falling back to Go. Protocols
// absent from this table keep their Go implementation, and historical tasks
// without a pinned plugin snapshot always run the Go path.
var seedanceExtensionProtocols = map[dto.VideoUpstreamProtocol][]string{
	dto.VideoUpstreamProtocolFeicaiVideosV1: {"buildCreate", "parseCreateResponse", "parseTaskObservation"},
}

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
	contract := pluginruntime.SeedanceExtensionContract{Key: SeedanceExtensionPluginKey}
	for protocol, hooks := range seedanceExtensionProtocols {
		contract.Protocols = append(contract.Protocols, pluginruntime.SeedanceExtensionProtocol{
			Name:  string(protocol),
			Hooks: hooks,
		})
	}
	return contract
}

// SeedanceExtensionProtocolMigrated reports whether new requests for this
// protocol are served by the extension plugin.
func SeedanceExtensionProtocolMigrated(protocol dto.VideoUpstreamProtocol) bool {
	_, migrated := seedanceExtensionProtocols[protocol]
	return migrated
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
	return seedanceExtensions.EnsureSeeded(ctx)
}

// SyncExtensionSnapshot publishes the active database rows to the extension
// store.
func SyncExtensionSnapshot(ctx context.Context, rows []model.TaskPlugin) error {
	return seedanceExtensions.SyncSnapshot(ctx, rows)
}

// DescribeExtension returns control-plane diagnostics for the extension.
func DescribeExtension() (activeVersion string, syncErrors []string) {
	return seedanceExtensions.Describe()
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
	_, migrated := seedanceExtensionProtocols[videoProtocol]
	if !migrated {
		return nil
	}
	plugin, err := seedanceExtensions.ActiveFor(videoProtocol)
	if err != nil {
		return err
	}
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	return nil
}
