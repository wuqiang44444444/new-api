package service

import (
	"net"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/jackc/pgx/v5"
)

// The local development launcher opts in. Every configured database endpoint,
// including a PostgreSQL failover host, must remain on this machine.
func localFastBillingReports() bool {
	if !common.GetEnvOrDefaultBool("LOCAL_DEV_FAST_BILLING_REPORTS", false) || !loopbackReportPostgres(os.Getenv("SQL_DSN")) {
		return false
	}
	logDSN := os.Getenv("LOG_SQL_DSN")
	return logDSN == "" || loopbackReportPostgres(logDSN)
}

func loopbackReportPostgres(dsn string) bool {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return false
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !reportLoopbackHost(config.Host) {
		return false
	}
	for _, fallback := range config.Fallbacks {
		if !reportLoopbackHost(fallback.Host) {
			return false
		}
	}
	return true
}

func reportLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
}
