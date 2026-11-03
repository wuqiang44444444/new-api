package model

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvidenceConcurrentCleanupWaitsForPublicationThenPreventsResurrection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "concurrent.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	previous, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { DB = previous; common.SetMainDatabaseType(previousType) })
	require.NoError(t, db.AutoMigrate(&TaskRequestEvidence{}))
	runEvidenceConcurrentPublicationCleanupContract(t)
}

func runEvidenceConcurrentPublicationCleanupContract(t *testing.T) {
	t.Helper()
	index := &TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, CreateTaskRequestEvidence(index))
	t.Cleanup(func() { require.NoError(t, DB.Delete(&TaskRequestEvidence{}, index.Id).Error) })
	path := filepath.Join(t.TempDir(), "segment.bin")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	published := make(chan error, 1)
	go func() {
		published <- CommitTaskRequestEvidenceObject(index.Id, 2*86400, func() error {
			close(entered)
			<-release
			return os.WriteFile(path, []byte("encrypted fixture"), 0600)
		})
	}()
	<-entered
	started, cleaned := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		err := RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp())
		if err == nil {
			err = os.Remove(path)
		}
		cleaned <- err
	}()
	<-started
	once.Do(func() { close(release) })
	require.NoError(t, <-published)
	require.NoError(t, <-cleaned, "cleanup cannot pass the shared lock before the pending publication")
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	assert.ErrorIs(t, CommitTaskRequestEvidenceObject(index.Id, 2*86400, func() error { return os.WriteFile(path, []byte("late fixture"), 0600) }), ErrTaskRequestEvidenceBodyClosed)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "late publication cannot resurrect deleted evidence")
}
