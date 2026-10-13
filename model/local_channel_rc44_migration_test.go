package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRC44ChannelMigrationPreservesFrozenFactsAndFunds(t *testing.T) {
	cases := []struct {
		name, env string
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "", sqlite.Open},
		{"mysql", "TEST_RC44_MYSQL_DSN", mysql.Open},
		{"postgres", "TEST_RC44_POSTGRES_DSN", postgres.Open},
	}
	for _, dialect := range cases {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.name == "sqlite" {
				dsn = filepath.Join(t.TempDir(), "rc44.sqlite")
			} else if dsn == "" {
				t.Skip("server database fixture is not configured")
			}
			db, err := gorm.Open(dialect.open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			oldDB := DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(common.DatabaseType(dialect.name), oldLog)
			t.Cleanup(func() {
				DB = oldDB
				common.SetDatabaseTypes(oldMain, oldLog)
				connection, err := db.DB()
				if err == nil {
					require.NoError(t, connection.Close())
				}
			})
			for _, table := range []any{&TaskCreateAttempt{}, &Task{}, &Channel{}, &Option{}} {
				require.False(t, db.Migrator().HasTable(table), "refusing a non-empty RC44 migration fixture")
			}
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&TaskCreateAttempt{}, &Task{}, &Channel{}, &Option{}))
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}, &Task{}, &TaskCreateAttempt{}))
			rows := []Channel{{Id: 1, Type: 62}, {Id: 2, Type: 63}, {Id: 3, Type: 64}, {Id: 4, Type: 66}, {Id: 5, Type: constant.ChannelTypeOpenAI}}
			require.NoError(t, db.Create(&rows).Error)
			task := &Task{TaskID: "rc44-frozen", Platform: "62", Status: TaskStatusInProgress, Quota: 101}
			require.NoError(t, db.Create(task).Error)
			raw := `{"southbound_adapter_version":"62:official:v1","image_task":{"channel_type":63,"future_image_fact":9},"future_fact":{"value":7}}`
			require.NoError(t, db.Model(task).Update("private_data", raw).Error)
			attempt := &TaskCreateAttempt{AttemptID: "rc44-held", AdapterVersion: "64:official:v1", Status: TaskCreateAttemptUnknown, BillingHoldState: TaskCreateAttemptBillingHeld, HeldQuota: 700}
			attempt.RecoverySnapshot = []byte(`{"task":{"platform":"64","quota":700},"private_data":{"southbound_adapter_version":"64:official:v1","future_fact":9}}`)
			require.NoError(t, db.Create(attempt).Error)
			require.NoError(t, migrateLocalChannelRC44Types())
			require.NoError(t, verifyLocalChannelRC44Types())
			var stored []Channel
			require.NoError(t, db.Order("id").Find(&stored).Error)
			got := []int{}
			for _, channel := range stored {
				got = append(got, channel.Type)
			}
			assert.Equal(t, []int{constant.ChannelTypeSeedanceLink, constant.ChannelTypeAsyncImage, constant.ChannelTypeMiniMaxLink, constant.ChannelTypeAzureBatch, constant.ChannelTypeOpenAI}, got)
			var saved Task
			require.NoError(t, db.First(&saved, task.ID).Error)
			assert.Equal(t, 101, saved.Quota)
			assert.Equal(t, constant.TaskPlatform("67"), saved.Platform)
			assert.Equal(t, TaskStatus(TaskStatusInProgress), saved.Status)
			var facts struct{ PrivateData string }
			require.NoError(t, db.Model(&Task{}).Select("private_data").Where("id = ?", task.ID).First(&facts).Error)
			assert.JSONEq(t, `{"southbound_adapter_version":"67:official:v1","image_task":{"channel_type":68,"future_image_fact":9},"future_fact":{"value":7}}`, facts.PrivateData)
			var savedAttempt TaskCreateAttempt
			require.NoError(t, db.First(&savedAttempt, attempt.ID).Error)
			assert.Equal(t, "69:official:v1", savedAttempt.AdapterVersion)
			assert.Equal(t, 700, savedAttempt.HeldQuota)
			assert.JSONEq(t, `{"task":{"platform":"69","quota":700},"private_data":{"southbound_adapter_version":"69:official:v1","future_fact":9}}`, string(savedAttempt.RecoverySnapshot))
			assert.Equal(t, TaskCreateAttemptBillingHeld, savedAttempt.BillingHoldState)
			assert.Equal(t, TaskCreateAttemptUnknown, savedAttempt.Status)
			native := Channel{Id: 6, Type: constant.ChannelTypeVLLM}
			require.NoError(t, db.Create(&native).Error)
			require.NoError(t, migrateLocalChannelRC44Types())
			require.NoError(t, db.First(&native, 6).Error)
			assert.Equal(t, constant.ChannelTypeVLLM, native.Type, "the marker protects newly created native channels from a second renumber")
		})
	}
}

func TestRC44ChannelMigrationRollsBackOnInvalidFrozenFacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rc44-rollback.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		connection, err := db.DB()
		if err == nil {
			require.NoError(t, connection.Close())
		}
	})
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}, &Task{}))
	channel := Channel{Type: 62}
	require.NoError(t, db.Create(&channel).Error)
	task := Task{TaskID: "invalid-frozen", Quota: 101}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Model(&task).Update("private_data", "{").Error)
	require.Error(t, migrateLocalChannelRC44Types())
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Equal(t, 62, channel.Type)
	var marker Option
	assert.ErrorIs(t, db.Where(&Option{Key: localChannelRC44MigrationKey}).First(&marker).Error, gorm.ErrRecordNotFound)
}
