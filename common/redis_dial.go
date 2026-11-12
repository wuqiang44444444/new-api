package common

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/go-redis/redis/v8"
)

func newRedisClient(options *redis.Options) *redis.Client {
	// go-redis v8's default TLS dialer ignores the operation context.
	if options.TLSConfig != nil && options.Dialer == nil {
		options.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := tls.Dialer{
				NetDialer: &net.Dialer{Timeout: options.DialTimeout, KeepAlive: 5 * time.Minute},
				Config:    options.TLSConfig,
			}
			return dialer.DialContext(ctx, network, addr)
		}
	}
	return redis.NewClient(options)
}
