package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLinkModelPublicationContractsSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	exerciseLinkModelPublicationDialect(t, db, common.DatabaseTypeSQLite)
}

func TestLinkModelPublicationContractsOnExternalDatabases(t *testing.T) {
	tests := []struct {
		name         string
		dsnEnv       string
		databaseType common.DatabaseType
		open         func(string) gorm.Dialector
	}{
		{name: "MySQL", dsnEnv: "TEST_LINK_CONTRACT_MYSQL_DSN", databaseType: common.DatabaseTypeMySQL, open: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "PostgreSQL", dsnEnv: "TEST_LINK_CONTRACT_POSTGRES_DSN", databaseType: common.DatabaseTypePostgreSQL, open: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.dsnEnv))
			if dsn == "" {
				t.Skipf("set %s to a disposable test database DSN", test.dsnEnv)
			}
			require.Contains(t, strings.ToLower(dsn), "_test", "external Link contract tests require a disposable *_test database")
			db, err := gorm.Open(test.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			exerciseLinkModelPublicationDialect(t, db, test.databaseType)
		})
	}
}

func exerciseLinkModelPublicationDialect(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(databaseType)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
	})

	require.NoError(t, db.AutoMigrate(
		&LinkModelPublication{},
		&LinkModelPublicationAudit{},
		&Task{},
		&TaskCreateAttempt{},
		&Asset{},
		&AssetBinding{},
		&ProviderCostExposure{},
	))

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	customerModel := "link-dialect-" + suffix
	publication, err := EnsureLinkModelPublication(db, LinkModelPublicationKey{
		RouteFamily:   LinkRouteFamilyModelArkVideo,
		CustomerModel: customerModel,
	}, VideoSKUSeedance20Standard720P, 101, 202, "dialect contract publication")
	require.NoError(t, err)
	publication, err = RebindLinkModelPublication(LinkModelPublicationKey{
		RouteFamily:   LinkRouteFamilyModelArkVideo,
		CustomerModel: customerModel,
	}, VideoSKUSeedance20Value720P, publication.PublicationVersion, 303, "dialect contract rebind")
	require.NoError(t, err)

	snapshot := LinkPubSnapshot{
		LinkContractNamespace:    publication.ContractNamespace,
		LinkRouteFamily:          string(publication.RouteFamily),
		PublishedLinkContractSKU: publication.LinkSKU,
		LinkPublicationVersion:   publication.PublicationVersion,
	}
	task := &Task{TaskID: "task_" + suffix, PrivateData: TaskPrivateData{LinkPubSnapshot: snapshot}}
	attempt := &TaskCreateAttempt{AttemptID: "attempt_" + suffix, PublicTaskID: task.TaskID, RequestHash: "request_" + suffix, LinkPubSnapshot: snapshot}
	asset := &Asset{PublicID: "ast_" + suffix, LinkPubSnapshot: snapshot}
	exposure := &ProviderCostExposure{SourceKind: ProviderCostExposureSourceTask, SourceID: task.TaskID, Reason: "dialect_test", LinkPubSnapshot: snapshot}
	for _, value := range []any{task, attempt, asset} {
		require.NoError(t, db.Create(value).Error)
	}
	binding := &AssetBinding{AssetID: asset.ID, LinkPubSnapshot: snapshot}
	require.NoError(t, db.Create(binding).Error)
	require.NoError(t, db.Create(exposure).Error)

	var savedTask Task
	require.NoError(t, db.First(&savedTask, "task_id = ?", task.TaskID).Error)
	assert.Equal(t, snapshot, savedTask.PrivateData.LinkPubSnapshot)
	var savedAttempt TaskCreateAttempt
	require.NoError(t, db.First(&savedAttempt, "attempt_id = ?", attempt.AttemptID).Error)
	assert.Equal(t, snapshot, savedAttempt.LinkPubSnapshot)
	var savedAsset Asset
	require.NoError(t, db.First(&savedAsset, "public_id = ?", asset.PublicID).Error)
	assert.Equal(t, snapshot, savedAsset.LinkPubSnapshot)
	var savedBinding AssetBinding
	require.NoError(t, db.First(&savedBinding, binding.ID).Error)
	assert.Equal(t, snapshot, savedBinding.LinkPubSnapshot)
	var savedExposure ProviderCostExposure
	require.NoError(t, db.First(&savedExposure, "source_id = ?", task.TaskID).Error)
	assert.Equal(t, snapshot, savedExposure.LinkPubSnapshot)

	var audits []LinkModelPublicationAudit
	require.NoError(t, db.Where("publication_id = ?", publication.ID).Order("publication_version asc").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, VideoSKUSeedance20Standard720P, audits[1].PreviousLinkSKU)
	assert.Equal(t, VideoSKUSeedance20Value720P, audits[1].LinkSKU)
}
