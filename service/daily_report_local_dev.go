//go:build yuan_gateway_local_dev

package service

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/jackc/pgx/v5"
)

func skipDailyReportResourceBudget() bool {
	if model.DB == nil || model.LOG_DB != model.DB {
		return false
	}
	dsn := os.Getenv("SQL_DSN")
	if !strings.HasPrefix(dsn, "postgresql://") && !strings.HasPrefix(dsn, "postgres://") {
		return false
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || len(config.Fallbacks) != 0 {
		return false
	}
	switch config.RuntimeParams["application_name"] {
	case "yuan-gateway-dev":
		return config.Host == "127.0.0.1" && config.Database == "yuan_gateway_dev"
	case "yuan-gateway-dev-cn":
		return config.Host == "127.0.0.1" && config.Database == "yuan_gateway_dev_cn"
	case "yuan-gateway-dev-compose":
		return config.Host == "postgres" && config.Port == 5432 && config.Database == "yuan_gateway_dev"
	default:
		return false
	}
}
