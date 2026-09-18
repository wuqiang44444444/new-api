package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBillingStatementProjectionDatabaseTypes(t *testing.T) {
	// Parse the actual migration schema without connecting to external databases.
	for _, tc := range []struct {
		name, want string
		dialect    gorm.Dialector
	}{
		{"mysql", "longtext", mysql.New(mysql.Config{DSN: "test@tcp(localhost:3306)/test", SkipInitializeWithVersion: true})},
		{"postgres", "text", postgres.Open("host=localhost user=test dbname=test sslmode=disable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(tc.dialect, &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			stmt := &gorm.Statement{DB: db}
			require.NoError(t, stmt.Parse(&BillingStatementVersion{}))
			for _, column := range []string{"summary_projection", "channel_projection", "dependencies"} {
				assert.Equal(t, tc.want, db.Migrator().FullDataTypeOf(stmt.Schema.FieldsByDBName[column]).SQL)
			}
		})
	}
}

func TestBillingStatementLargeFrozenProjectionRoundTrip(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	statement := BillingCustomerStatement{}
	group := BillingReconciliationGroupSummary{Id: 7, Name: "customer-key"}
	for i := 0; i < 1000; i++ {
		group.Models = append(group.Models, BillingReconciliationModelSummary{ModelName: fmt.Sprintf("customer-model-%04d", i), BillingMode: BillingReconciliationModePerCall, Usage: BillingReconciliationUsage{Requests: 1, NetQuota: 10}})
	}
	statement.Groups = []BillingReconciliationGroupSummary{group}
	raw, err := FreezeBillingStatementProjection(statement)
	require.NoError(t, err)
	require.Greater(t, len(raw), 65535, "valid multi-model statement exceeds MySQL TEXT capacity")
	v := BillingStatementVersion{DraftPublicId: "large-projection", SummaryProjection: raw, ChannelProjection: raw, Dependencies: "{}"}
	require.NoError(t, db.Create(&v).Error)
	var read BillingStatementVersion
	require.NoError(t, db.First(&read, v.ID).Error)
	require.Equal(t, raw, read.SummaryProjection)
	require.Equal(t, raw, read.ChannelProjection)
	projection, err := BillingStatementVersionStatement(&read)
	require.NoError(t, err)
	require.Len(t, projection.Groups, 1)
	assert.Len(t, projection.Groups[0].Models, 1000)
}

func TestBillingStatementProjectionMigrationPreservesExistingRows(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	row := BillingStatementVersion{DraftPublicId: "migration-existing", SummaryProjection: `{"fixture":"preserved"}`, ChannelProjection: `{"fixture":"preserved"}`, Dependencies: "{}"}
	require.NoError(t, db.Create(&row).Error)
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// Simulate the original TEXT schema on the isolated test database only.
		require.NoError(t, db.Exec("ALTER TABLE billing_statement_versions MODIFY summary_projection TEXT, MODIFY channel_projection TEXT, MODIFY dependencies TEXT").Error)
	}
	require.NoError(t, db.AutoMigrate(&BillingStatementVersion{}))
	require.NoError(t, db.AutoMigrate(&BillingStatementVersion{}))
	var restored BillingStatementVersion
	require.NoError(t, db.First(&restored, row.ID).Error)
	assert.Equal(t, row.SummaryProjection, restored.SummaryProjection)
	large := BillingStatementJSON(`{"fixture":"` + strings.Repeat("x", 70000) + `"}`)
	require.NoError(t, db.Model(&restored).Updates(map[string]any{"summary_projection": large, "channel_projection": large, "dependencies": large}).Error)
	require.NoError(t, db.First(&restored, row.ID).Error)
	assert.Equal(t, large, restored.SummaryProjection)
	assert.Equal(t, large, restored.ChannelProjection)
	assert.Equal(t, large, restored.Dependencies)
}
