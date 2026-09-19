package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Opt-in server verification always creates a new database. It never migrates
// or drops tables from the database named by the connection configuration.
func setupBillingStatementServerDB(t testing.TB, dialect string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_BILLING_STATEMENT_DSN")
	require.NotEmpty(t, dsn)
	name := "yuan_bsv_" + strings.ReplaceAll(common.GetUUID(), "-", "")
	var root, db *gorm.DB
	var err error
	switch dialect {
	case "mysql":
		config, parseErr := mysqldriver.ParseDSN(dsn)
		require.NoError(t, parseErr)
		require.Equal(t, "tcp", config.Net)
		require.True(t, strings.HasPrefix(config.Addr, "127.0.0.1:"), "server acceptance requires an isolated loopback database")
		root, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, root.Exec("CREATE DATABASE "+name+" CHARACTER SET utf8mb4").Error)
		config.DBName = name
		db, err = gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	case "postgres":
		config, parseErr := pgx.ParseConfig(dsn)
		require.NoError(t, parseErr)
		require.Equal(t, "127.0.0.1", config.Host, "server acceptance requires an isolated loopback database")
		root, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, root.Exec("CREATE DATABASE "+name).Error)
		config.Database = name
		db, err = gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}), &gorm.Config{})
	default:
		t.Fatalf("unsupported isolated test database: %s", dialect)
	}
	t.Cleanup(func() {
		if db != nil {
			if conn, e := db.DB(); e == nil {
				_ = conn.Close()
			}
		}
		require.NoError(t, root.Exec("DROP DATABASE "+name).Error)
		conn, e := root.DB()
		require.NoError(t, e)
		_ = conn.Close()
	})
	require.NoError(t, err)
	return db
}
