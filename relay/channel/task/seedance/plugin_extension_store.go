package seedance

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

var (
	errSeedanceExtensionUnavailable = errors.New("the seedance-link extension plugin is unavailable for the requested protocol")

	// ErrSeedanceExtensionVersionUnavailable is returned when the exact plugin
	// version frozen on a task cannot be resolved from the version store.
	// Callers treat it as fail-closed (reconciliation), never as a refund or
	// a fallback to the active version.
	ErrSeedanceExtensionVersionUnavailable = errors.New("the frozen seedance-link plugin version is unavailable")
)

type seedanceExtensionEntry struct {
	version    string
	sourceHash string
	plugin     *pluginruntime.LoadedPlugin
	info       pluginruntime.SeedanceExtensionInfo
}

type seedanceExtensionStore struct {
	mu         sync.RWMutex
	compiled   map[string]*seedanceExtensionEntry
	active     *seedanceExtensionEntry
	seeded     map[string]bool
	syncErrors []string
}

var seedanceExtensions = &seedanceExtensionStore{
	compiled: make(map[string]*seedanceExtensionEntry),
	seeded:   make(map[string]bool),
}

func sourceHashOf(source string) string {
	return fmt.Sprintf("%x", common.Sha256Raw([]byte(source)))
}

// EnsureSeeded inserts the embedded extension artifact into the version
// store when the exact version is absent. The first version of the key
// becomes active through SaveTaskPlugin semantics; later embedded versions
// are seeded non-active so activation stays an explicit administrator
// action. Seeding is marked done only after the version is confirmed
// persisted, so a transient database failure retries on the next sync.
func (s *seedanceExtensionStore) EnsureSeeded(ctx context.Context) error {
	// The embedded artifact cannot change during this process. Check completed
	// seeding before compiling; only successful persistence sets this marker.
	s.mu.RLock()
	seeded := len(s.seeded) != 0
	s.mu.RUnlock()
	if seeded {
		return nil
	}
	source := plugins.SeedanceSource()
	plugin, _, err := CompileSeedanceExtensionSource(source)
	if err != nil {
		return fmt.Errorf("embedded seedance-link artifact is invalid: %w", err)
	}
	version := plugin.Meta.Version

	existing, err := model.GetTaskPluginVersion(SeedanceExtensionPluginKey, version)
	if err == nil && existing != nil {
		s.markSeeded(version)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	seed := model.TaskPlugin{
		Key:        SeedanceExtensionPluginKey,
		APIVersion: plugin.Meta.APIVersion,
		Version:    version,
		Source:     source,
		SourceHash: sourceHashOf(source),
		Enabled:    true,
		Remark:     "embedded Seedance Link extension artifact",
	}
	if err = model.SaveTaskPlugin(&seed); err != nil {
		return fmt.Errorf("seed embedded seedance-link artifact %s: %w", version, err)
	}
	s.markSeeded(version)
	return nil
}

// markSeeded records a completed seed. Once set, the process does not seed
// the same version again, so deleting a dependency-free version is not
// undone by the sync loop itself.
func (s *seedanceExtensionStore) markSeeded(version string) {
	s.mu.Lock()
	s.seeded[version] = true
	s.mu.Unlock()
}

// SyncSnapshot publishes only validated active artifacts. Failed compilation
// is visible and stops new admission; it never silently substitutes cached code.
// Objects already pinned by running requests are not modified.
func (s *seedanceExtensionStore) SyncSnapshot(ctx context.Context, rows []model.TaskPlugin) error {
	var active *seedanceExtensionEntry
	var syncErrors []string
	for i := range rows {
		row := rows[i]
		if row.Key != SeedanceExtensionPluginKey {
			continue
		}
		entry, entryErr := s.compileRow(&row)
		if entryErr != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("seedance-link %s: %v", row.Version, entryErr))
			continue
		}
		if row.Active {
			active = entry
		}
	}
	s.mu.Lock()
	s.active = active
	if len(syncErrors) > 0 {
		s.syncErrors = syncErrors
	} else {
		s.syncErrors = nil
	}
	s.mu.Unlock()
	return nil
}

func (s *seedanceExtensionStore) compileRow(row *model.TaskPlugin) (*seedanceExtensionEntry, error) {
	s.mu.RLock()
	entry := s.compiled[row.Version]
	s.mu.RUnlock()
	if entry != nil && entry.sourceHash == row.SourceHash {
		return entry, nil
	}
	plugin, info, err := CompileSeedanceExtensionSource(row.Source)
	if err != nil {
		return nil, err
	}
	if plugin.Meta.Version != row.Version {
		return nil, fmt.Errorf("artifact meta version %s does not match stored version %s", plugin.Meta.Version, row.Version)
	}
	next := &seedanceExtensionEntry{version: row.Version, sourceHash: row.SourceHash, plugin: plugin, info: info}
	s.mu.Lock()
	s.compiled[row.Version] = next
	s.mu.Unlock()
	return next, nil
}

// ActiveFor returns the active compiled extension when it declares the
// migrated protocol. New requests fail closed otherwise.
func (s *seedanceExtensionStore) ActiveFor(protocol dto.VideoUpstreamProtocol) (*pluginruntime.LoadedPlugin, error) {
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if active == nil {
		return nil, errSeedanceExtensionUnavailable
	}
	if !seedanceInfoDeclaresProtocol(active.info, string(protocol)) {
		return nil, errSeedanceExtensionUnavailable
	}
	return active.plugin, nil
}

// ResolveVersion compiles (or reuses) the exact plugin version frozen on a
// task. The database row is always read first so deleted versions stop
// resolving; disabled-but-present versions keep resolving, which separates
// "stop accepting new requests" from "stop executing history".
func (s *seedanceExtensionStore) ResolveVersion(ctx context.Context, version string) (*pluginruntime.LoadedPlugin, error) {
	row, err := model.GetTaskPluginVersion(SeedanceExtensionPluginKey, version)
	if err != nil || row == nil {
		return nil, ErrSeedanceExtensionVersionUnavailable
	}
	entry, entryErr := s.compileRow(row)
	if entryErr != nil {
		return nil, ErrSeedanceExtensionVersionUnavailable
	}
	return entry.plugin, nil
}

// Describe returns control-plane diagnostics.
func (s *seedanceExtensionStore) Describe() (activeVersion string, errors []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active != nil {
		activeVersion = s.active.version
	}
	if len(s.syncErrors) > 0 {
		errors = append([]string(nil), s.syncErrors...)
	}
	return activeVersion, errors
}

// seedanceInfoDeclaresProtocol reports whether the compiled artifact
// implements the named Seedance protocol.
func seedanceInfoDeclaresProtocol(info pluginruntime.SeedanceExtensionInfo, protocol string) bool {
	for _, name := range info.Protocols {
		if name == protocol {
			return true
		}
	}
	return false
}
