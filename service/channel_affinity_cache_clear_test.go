package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultChannelAffinityClearAll(t *testing.T) {
	t.Run("memory", func(t *testing.T) { testAffinityClearAll(t, nil) })
	t.Run("redis", func(t *testing.T) {
		url := os.Getenv("YUAN_AFFINITY_TEST_REDIS")
		if url == "" {
			t.Skip("set YUAN_AFFINITY_TEST_REDIS to the disposable ./start.sh --redis-only endpoint")
		}
		options, err := redis.ParseURL(url)
		require.NoError(t, err)
		client := redis.NewClient(options)
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, client.Ping(ctx).Err())
		testAffinityClearAll(t, client)

		// A real Redis pause delays commands on an established connection.
		cache := defaultAffinityCache()
		require.NoError(t, cache.SetWithTTL("recovery", 31, time.Minute))
		require.NoError(t, client.Do(ctx, "CLIENT", "PAUSE", 250, "ALL").Err())
		start := time.Now()
		_, _, err = cache.Get("recovery")
		require.Error(t, err)
		t.Logf("Redis paused read: %s", time.Since(start))
		assert.Less(t, time.Since(start), 150*time.Millisecond)
		start = time.Now()
		require.Error(t, cache.SetWithTTL("paused", 32, time.Minute))
		t.Logf("Redis paused write: %s", time.Since(start))
		assert.Less(t, time.Since(start), 150*time.Millisecond)
		require.Eventually(t, func() bool {
			id, found, err := cache.Get("recovery")
			return err == nil && found && id == 31
		}, time.Second, 25*time.Millisecond)
	})
}

func testAffinityClearAll(t *testing.T, client *redis.Client) {
	t.Helper()
	setupDefaultAffinityTest(t, client)
	previousLegacy := getChannelAffinityCache()
	channelAffinityCache = cachex.NewHybridCache(cachex.HybridCacheConfig[int]{
		Namespace: channelAffinityCacheNamespace, Redis: client, RedisCodec: cachex.IntCodec{},
	})
	t.Cleanup(func() { channelAffinityCache = previousLegacy })
	setting := operation_setting.GetChannelAffinitySetting()
	setting.Rules = []operation_setting.ChannelAffinityRule{{Name: "fixture", IncludeRuleName: true}}

	legacy, defaults := getChannelAffinityCache(), defaultAffinityCache()
	require.NoError(t, legacy.SetWithTTL("fixture:session", 11, time.Minute))
	require.NoError(t, defaults.SetWithTTL("default-user", 21, time.Minute))
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, client.Set(ctx, "unrelated-fixture", "keep", time.Minute).Err())
	}

	deleted, err := ClearChannelAffinityCacheByRuleName("fixture")
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
	_, found, err := defaults.Get("default-user")
	require.NoError(t, err)
	require.True(t, found)

	require.NoError(t, legacy.SetWithTTL("fixture:session", 11, time.Minute))
	deleted, err = ClearChannelAffinityCacheAll()
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)
	for _, cache := range []*cachex.HybridCache[int]{legacy, defaults} {
		keys, err := cache.Keys()
		require.NoError(t, err)
		assert.Empty(t, keys)
	}
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		value, err := client.Get(ctx, "unrelated-fixture").Result()
		require.NoError(t, err)
		assert.Equal(t, "keep", value)
	}
}
