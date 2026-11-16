package model

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Read the recorded verdict only for channel tests. Historical events without
// a verdict still contribute to the daily error report.
func withoutPassedChannelTests(query *gorm.DB) *gorm.DB {
	var result string
	switch common.LogDatabaseType() {
	case common.DatabaseTypeMySQL:
		result = "JSON_UNQUOTE(JSON_EXTRACT(NULLIF(detail, ''), '$.check_result'))"
	case common.DatabaseTypePostgreSQL:
		result = "(NULLIF(detail, '')::json ->> 'check_result')"
	case common.DatabaseTypeClickHouse:
		result = "JSONExtractString(detail, 'check_result')"
	default:
		result = "json_extract(NULLIF(detail, ''), '$.check_result')"
	}
	return query.Where("CASE WHEN event_type = ? THEN COALESCE("+result+", '') <> ? ELSE TRUE END",
		clienterrlog.EventChannelTest, "passed")
}
