package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// filterErrorEventHTTPStatus matches the status shown by ErrorLogStatus:
// channel tests use upstream_status; other events use the actual client status.
// Detail is either empty or JSON encoded by persistErrorEvent. Extracting its
// top-level field also works for historical rows without http_exchange, without
// rewriting them or filtering after pagination. Each log dialect needs its own
// JSON extraction syntax; no JSON column type or schema migration is required.
func filterErrorEventHTTPStatus(query *gorm.DB, status int) *gorm.DB {
	var value, valid string
	switch common.LogDatabaseType() {
	case common.DatabaseTypeMySQL:
		value = "JSON_UNQUOTE(JSON_EXTRACT(NULLIF(detail, ''), '$.upstream_status'))"
		valid = value + " REGEXP '^[1-5][0-9]{2}$'"
	case common.DatabaseTypePostgreSQL:
		value = "(NULLIF(detail, '')::json ->> 'upstream_status')"
		valid = value + " ~ '^[1-5][0-9]{2}$'"
	case common.DatabaseTypeClickHouse:
		value = "JSONExtractString(detail, 'upstream_status')"
		valid = "match(" + value + ", '^[1-5][0-9]{2}$')"
	default: // SQLite; the bundled driver includes JSON functions.
		value = "CAST(json_extract(NULLIF(detail, ''), '$.upstream_status') AS TEXT)"
		valid = value + " GLOB '[1-5][0-9][0-9]'"
	}
	// Compare canonical strings so invalid/absent statuses become 0 without an
	// unsafe numeric cast. Backend diagnostics serialize upstream_status as text.
	upstream := "CASE WHEN " + valid + " THEN " + value + " ELSE '0' END"
	return query.Where("((event_type = ? AND ("+upstream+") = ?) OR (event_type <> ? AND status = ?))",
		clienterrlog.EventChannelTest, fmt.Sprint(status), clienterrlog.EventChannelTest, status)
}
