package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAffinityCacheDetailsReportsLiveRedactedBindings(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.Create(&model.Channel{Id: 701, Name: "Primary", Key: "fixture-upstream-secret"}).Error)
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var client *redis.Client
			var redisServer *miniredis.Miniredis
			if backend == "redis" {
				redisServer = miniredis.RunT(t)
				client = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
				t.Cleanup(func() { require.NoError(t, client.Close()) })
			}
			setupDefaultAffinityTest(t, client)
			previousSession, previousUser := getChannelAffinityCache(), userAffinityCache
			channelAffinityCache = cachex.NewInspectableHybridCache(cachex.HybridCacheConfig[int]{
				Namespace: channelAffinityCacheNamespace, Redis: client, RedisCodec: cachex.IntCodec{},
			}, func() *hot.HotCache[string, cachex.InspectableValue[int]] {
				return hot.NewHotCache[string, cachex.InspectableValue[int]](hot.LRU, 100).Build()
			})
			userCache := newDefaultAffinityCache(nil)
			if client != nil {
				userCache = cachex.NewHybridCache(cachex.HybridCacheConfig[int]{
					Namespace: "fixture:user-affinity", Redis: client, RedisCodec: cachex.IntCodec{},
				})
			}
			userAffinityCache = func() *cachex.HybridCache[int] { return userCache }
			t.Cleanup(func() { channelAffinityCache, userAffinityCache = previousSession, previousUser })

			for _, layer := range []string{"session", "api_key", "user"} {
				cache := channelAffinityCacheForLayer(layer)
				rawKey := "fixture:private-" + layer
				require.NoError(t, cache.SetWithTTL(rawKey, 701, 2*time.Minute))
				expiringTTL := -time.Second
				if redisServer != nil {
					expiringTTL = time.Second
				}
				require.NoError(t, cache.SetWithTTL("expired", 702, expiringTTL))
			}
			if redisServer != nil {
				redisServer.FastForward(30 * time.Second)
			}

			for _, layer := range []string{"session", "api_key", "user"} {
				page, err := GetChannelAffinityCacheDetails(t.Context(), layer, 0)
				require.NoError(t, err)
				require.Len(t, page.Entries, 1, layer)
				entry := page.Entries[0]
				assert.Equal(t, 701, entry.ChannelID)
				assert.Equal(t, "Primary", entry.ChannelName)
				assert.Regexp(t, "^[0-9a-f]{16}$", entry.Fingerprint)
				require.NotNil(t, entry.TTLSeconds)
				assert.Positive(t, *entry.TTLSeconds)
				assert.LessOrEqual(t, *entry.TTLSeconds, int64(120))
				raw, err := common.Marshal(page)
				require.NoError(t, err)
				assert.NotContains(t, string(raw), "fixture:")
				assert.NotContains(t, string(raw), "fixture-upstream-secret")
				if redisServer != nil {
					assert.EqualValues(t, 90, *entry.TTLSeconds)
					assert.Equal(t, 90*time.Second, redisServer.TTL(channelAffinityCacheForLayer(layer).FullKey("fixture:private-"+layer)))
				}
			}
		})
	}
}
