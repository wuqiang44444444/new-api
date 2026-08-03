package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDropLegacyRealPersonConsentSchemaRemovesTableAndColumns(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previousDB })

	require.NoError(t, db.Exec(`CREATE TABLE consent_policies (id integer primary key, version text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE real_person_authorizations (
		id integer primary key,
		public_id text,
		status text,
		policy_id integer,
		policy_hash text,
		locale text,
		adult_confirmed numeric,
		consented_at integer,
		consent_evidence_hmac text,
		user_agent text,
		consent_token_hash text,
		receipt_token_hash text,
		consent_token_expires_at integer
	)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_real_person_authorizations_policy_id ON real_person_authorizations(policy_id)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_real_person_authorizations_consent_token_hash ON real_person_authorizations(consent_token_hash)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_real_person_authorizations_receipt_token_hash ON real_person_authorizations(receipt_token_hash)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_real_person_authorizations_consent_token_expires_at ON real_person_authorizations(consent_token_expires_at)`).Error)

	require.NoError(t, dropLegacyRealPersonConsentSchema())
	assert.False(t, db.Migrator().HasTable("consent_policies"))
	columns, err := db.Migrator().ColumnTypes("real_person_authorizations")
	require.NoError(t, err)
	remaining := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		remaining[column.Name()] = struct{}{}
	}
	for _, column := range legacyRealPersonConsentColumns {
		_, exists := remaining[column]
		assert.False(t, exists, column)
	}
	_, hasStatus := remaining["status"]
	assert.True(t, hasStatus)
	require.NoError(t, dropLegacyRealPersonConsentSchema())
}
