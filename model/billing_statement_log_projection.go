package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// group is reserved in all supported SQL dialects. The alias keeps scans
// independent of the source column's quoting and nullable historical values.
func billingStatementGroupSelect() string {
	column := "`group`"
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		column = `"group"`
	}
	return "COALESCE(" + column + ", '') AS group_name"
}

// Attach only the shared customer-safe projection before role redaction.
// Recovered facts never copy admin snapshots, upstream identities or source
// references into the response, and never update the persisted log.
func attachBillingStatementLogFacts(log *Log, fact billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil || other == nil {
		other = make(map[string]json.RawMessage)
	}
	row := buildCustomerExportRow(customerBillingLogScanRow(log), fact, parsed)
	raw, err := common.Marshal(row)
	if err != nil {
		return err
	}
	other["billing_facts"] = raw
	encoded, err := common.Marshal(other)
	if err != nil {
		return err
	}
	log.Other = string(encoded)
	log.Group = row.GroupName
	return nil
}
