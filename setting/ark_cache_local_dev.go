//go:build yuan_gateway_local_dev

package setting

import (
	"os"
	"regexp"
	"sync"

	"github.com/jackc/pgx/v5"
)

var arkCacheIsolationSchema = regexp.MustCompile(`^-c search_path=ark_session_local_[a-f0-9]{12}$`)

var arkCacheIsolatedLocalDatabase = sync.OnceValue(func() bool {
	options := os.Getenv("PGOPTIONS")
	if !arkCacheIsolationSchema.MatchString(options) {
		return false
	}
	config, err := pgx.ParseConfig(os.Getenv("SQL_DSN"))
	return err == nil && config.Host == "127.0.0.1" && len(config.Fallbacks) == 0 &&
		config.Database == "yuan_gateway_dev" && config.RuntimeParams["application_name"] == "yuan-gateway-dev" &&
		config.RuntimeParams["search_path"] == "" && config.RuntimeParams["options"] == options
})

func ArkCacheIsolatedLocalDatabase() bool { return arkCacheIsolatedLocalDatabase() }
