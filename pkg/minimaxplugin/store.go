// Package minimaxplugin owns the compiled-version store of the minimax-link
// typed extension artifact. It mirrors the seedanceplugin store but with a
// deploy-only seeding lifecycle: the embedded artifact is inserted without
// activation, and an administrator must explicitly activate a version before
// the MiniMax Link channel type admits new requests.
package minimaxplugin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"gorm.io/gorm"
)

var (
	ErrUnavailable = errors.New("the minimax-link extension plugin is unavailable for the requested protocol")

	// ErrVersionUnavailable is returned when the exact plugin version frozen
	// on a task cannot be resolved from the version store. Callers treat it
	// as fail-closed (reconciliation), never as a refund or a fallback to the
	// active version.
	ErrVersionUnavailable = errors.New("the frozen minimax-link plugin version is unavailable")
)

type CompiledVersion struct {
	version    string
	sourceHash string
	Plugin     *pluginruntime.LoadedPlugin
	Info       pluginruntime.SeedanceExtensionInfo
}

type Store struct {
	mu         sync.RWMutex
	compiled   map[string]*CompiledVersion
	active     *CompiledVersion
	seeded     map[string]bool
	syncErrors []string
}

// Default is the compiled-version store for the minimax-link artifact.
var Default = NewStore()

func NewStore() *Store {
	return &Store{compiled: make(map[string]*CompiledVersion), seeded: make(map[string]bool)}
}

func sourceHashOf(source string) string {
	return fmt.Sprintf("%x", common.Sha256Raw([]byte(source)))
}

// EnsureSeeded inserts the embedded artifact into the version store when
// absent. Deployment never enables the version: new admission stays closed
// until an administrator explicitly enables it. The first stored version may
// become active through the existing SaveTaskPlugin rule; with Enabled=false
// it still admits no requests. Later embedded versions only fill missing
// records — an existing row is verified byte-for-byte and never re-saved, so
// administrator enable/disable choices and the active selection are not
// overwritten, and no auto-promotion ever replaces the current active.
// Re-seeding a deleted version is not done; an existing row with different
// bytes is a hard error.
func (s *Store) EnsureSeeded(ctx context.Context) error {
	s.mu.RLock()
	seeded := len(s.seeded) != 0
	s.mu.RUnlock()
	if seeded {
		return nil
	}
	source := plugins.MinimaxSource()
	plugin, _, err := pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{}, pluginruntime.MinimaxHostContract())
	if err != nil {
		return fmt.Errorf("embedded minimax-link artifact is invalid: %w", err)
	}
	version := plugin.Meta.Version

	existing, err := model.GetTaskPluginVersion(pluginruntime.MinimaxPluginKey, version)
	if err == nil && existing != nil {
		if existing.Source != source || existing.APIVersion != plugin.Meta.APIVersion {
			return errors.New("embedded minimax-link version conflicts with its stored artifact; publish a new version")
		}
		s.markSeeded(version)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	seed := model.TaskPlugin{
		Key:        pluginruntime.MinimaxPluginKey,
		APIVersion: plugin.Meta.APIVersion,
		Version:    version,
		Source:     source,
		SourceHash: sourceHashOf(source),
		Enabled:    false,
		Remark:     "embedded MiniMax Link extension artifact",
	}
	if err = model.SaveTaskPlugin(&seed); err != nil {
		return fmt.Errorf("seed embedded minimax-link artifact %s: %w", version, err)
	}
	s.markSeeded(version)
	return nil
}

// markSeeded records a completed seed. Once set, the process does not seed
// the same version again, so deleting a dependency-free version is not
// undone by the sync loop itself.
func (s *Store) markSeeded(version string) {
	s.mu.Lock()
	s.seeded[version] = true
	s.mu.Unlock()
}

// SyncSnapshot publishes only the validated active artifact. Failed
// compilation is visible and stops new admission; it never silently
// substitutes cached code.
func (s *Store) SyncSnapshot(ctx context.Context, rows []model.TaskPlugin) error {
	var active *CompiledVersion
	var syncErrors []string
	for i := range rows {
		row := rows[i]
		if row.Key != pluginruntime.MinimaxPluginKey {
			continue
		}
		entry, entryErr := s.compileRow(&row)
		if entryErr != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("minimax-link %s: %v", row.Version, entryErr))
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

func (s *Store) compileRow(row *model.TaskPlugin) (*CompiledVersion, error) {
	s.mu.RLock()
	entry := s.compiled[row.Version]
	s.mu.RUnlock()
	if entry != nil && entry.sourceHash == row.SourceHash {
		return entry, nil
	}
	plugin, info, err := pluginruntime.CompileSeedanceExtension(row.Source, pluginruntime.Options{}, pluginruntime.MinimaxHostContract())
	if err != nil {
		return nil, err
	}
	if plugin.Meta.Version != row.Version {
		return nil, fmt.Errorf("artifact meta version %s does not match stored version %s", plugin.Meta.Version, row.Version)
	}
	next := &CompiledVersion{version: row.Version, sourceHash: row.SourceHash, Plugin: plugin, Info: info}
	s.mu.Lock()
	s.compiled[row.Version] = next
	s.mu.Unlock()
	return next, nil
}

// ActiveEntryFor resolves code and its declaration under the same snapshot
// lock. New requests fail closed when no activated version covers the
// protocol. A request must never read the active declaration again after
// this pin.
func (s *Store) ActiveEntryFor(protocol string) (*CompiledVersion, error) {
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if active == nil || !infoDeclaresProtocol(active.Info, protocol) {
		return nil, ErrUnavailable
	}
	return active, nil
}

// ResolveVersion compiles (or reuses) the exact plugin version frozen on a
// task. The database row is always read first so deleted versions stop
// resolving; disabled-but-present versions keep resolving, which separates
// "stop accepting new requests" from "stop executing history".
func (s *Store) ResolveVersion(ctx context.Context, version string) (*pluginruntime.LoadedPlugin, error) {
	row, err := model.GetTaskPluginVersion(pluginruntime.MinimaxPluginKey, version)
	if err != nil || row == nil {
		return nil, ErrVersionUnavailable
	}
	entry, entryErr := s.compileRow(row)
	if entryErr != nil {
		return nil, ErrVersionUnavailable
	}
	return entry.Plugin, nil
}

// Describe returns control-plane diagnostics.
func (s *Store) Describe() (activeVersion string, errors []string) {
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

// infoDeclaresProtocol reports whether the compiled artifact implements the
// named southbound protocol.
func infoDeclaresProtocol(info pluginruntime.SeedanceExtensionInfo, protocol string) bool {
	for _, name := range info.Protocols {
		if name == protocol {
			return true
		}
	}
	return false
}
