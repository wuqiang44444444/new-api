package model

import "fmt"

var legacyRealPersonConsentColumns = []string{
	"policy_id",
	"policy_hash",
	"locale",
	"adult_confirmed",
	"consented_at",
	"consent_evidence_hmac",
	"user_agent",
	"consent_token_hash",
	"receipt_token_hash",
	"consent_token_expires_at",
}

var legacyRealPersonConsentIndexes = []string{
	"idx_real_person_authorizations_policy_id",
	"idx_real_person_authorizations_consent_token_hash",
	"idx_real_person_authorizations_receipt_token_hash",
	"idx_real_person_authorizations_consent_token_expires_at",
}

type legacyRealPersonConsentSchema struct{}

func (legacyRealPersonConsentSchema) TableName() string { return "real_person_authorizations" }

// dropLegacyRealPersonConsentSchema permanently removes the retired platform
// consent implementation. APIServiceRule is the only application agreement;
// Provider H5 is the only real-person verification flow.
func dropLegacyRealPersonConsentSchema() error {
	migrator := DB.Migrator()
	if migrator.HasTable("consent_policies") {
		if err := migrator.DropTable("consent_policies"); err != nil {
			return fmt.Errorf("drop legacy consent policies: %w", err)
		}
	}
	const authorizationTable = "real_person_authorizations"
	if !migrator.HasTable(authorizationTable) {
		return nil
	}
	columnTypes, err := migrator.ColumnTypes(authorizationTable)
	if err != nil {
		return fmt.Errorf("inspect real-person authorization schema: %w", err)
	}
	present := make(map[string]struct{}, len(columnTypes))
	for _, columnType := range columnTypes {
		present[columnType.Name()] = struct{}{}
	}
	for _, index := range legacyRealPersonConsentIndexes {
		if migrator.HasIndex(&legacyRealPersonConsentSchema{}, index) {
			if err := migrator.DropIndex(&legacyRealPersonConsentSchema{}, index); err != nil {
				return fmt.Errorf("drop legacy real-person consent index %s: %w", index, err)
			}
		}
	}
	quote := "`"
	if DB.Dialector.Name() == "postgres" {
		quote = `"`
	}
	for _, column := range legacyRealPersonConsentColumns {
		if _, exists := present[column]; !exists {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s%s%s DROP COLUMN %s%s%s", quote, authorizationTable, quote, quote, column, quote)
		if err := DB.Exec(statement).Error; err != nil {
			return fmt.Errorf("drop legacy real-person consent column %s: %w", column, err)
		}
	}
	return nil
}
