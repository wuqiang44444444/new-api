package cachex

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
)

// InspectableValue keeps a value and its expiry together through eviction and
// replacement. HotCache does not expose the remaining TTL.
type InspectableValue[V any] struct {
	value     V
	expiresAt time.Time
}

type inspectableMemory[V any] struct {
	*hot.HotCache[string, InspectableValue[V]]
}

func (m inspectableMemory[V]) Get(key string) (V, bool, error) {
	entry, found, err := m.HotCache.Get(key)
	return entry.value, found, err
}

func (m inspectableMemory[V]) SetWithTTL(key string, value V, ttl time.Duration) {
	entry := InspectableValue[V]{value: value}
	if ttl != 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	m.HotCache.SetWithTTL(key, entry, ttl)
}

func NewInspectableHybridCache[V any](cfg HybridCacheConfig[V], memory func() *hot.HotCache[string, InspectableValue[V]]) *HybridCache[V] {
	cache := NewHybridCache(cfg)
	cache.memInit = func() memoryStore[V] { return inspectableMemory[V]{memory()} }
	return cache
}

type InspectedEntry[V any] struct {
	Key   string
	Value V
	TTL   time.Duration // -1 means the binding has no expiry.
}

const inspectPageSize = 50

// Inspect reads one page without refreshing entries. A nonzero cursor means
// there may be more entries. Pages reflect changes to the live cache.
func (c *HybridCache[V]) Inspect(ctx context.Context, cursor uint64) ([]InspectedEntry[V], uint64, error) {
	if c.redisOn() {
		return c.inspectRedis(ctx, cursor)
	}
	memory, ok := c.memCache().(inspectableMemory[V])
	if !ok {
		return nil, 0, errors.New("cache does not expose expiry")
	}
	keys := memory.Keys()
	if cursor >= uint64(len(keys)) {
		return nil, 0, nil
	}
	slices.Sort(keys)
	now := time.Now()
	entries := make([]InspectedEntry[V], 0, inspectPageSize)
	for ; cursor < uint64(len(keys)) && len(entries) < inspectPageSize; cursor++ {
		key := keys[cursor]
		entry, found := memory.Peek(key)
		if !found {
			continue
		}
		ttl := time.Duration(-1)
		if !entry.expiresAt.IsZero() {
			ttl = entry.expiresAt.Sub(now)
			if ttl <= 0 {
				continue
			}
		}
		entries = append(entries, InspectedEntry[V]{Key: key, Value: entry.value, TTL: ttl})
	}
	if cursor == uint64(len(keys)) {
		cursor = 0
	}
	return entries, cursor, nil
}

func (c *HybridCache[V]) inspectRedis(ctx context.Context, cursor uint64) ([]InspectedEntry[V], uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultRedisOpTimeout)
	defer cancel()
	keys := make([]string, 0, inspectPageSize)
	for range 16 {
		batch, next, err := c.redis.Scan(ctx, cursor, c.ns.MatchPattern(), inspectPageSize).Result()
		if err != nil {
			return nil, 0, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 || len(keys) >= inspectPageSize {
			break
		}
	}
	if len(keys) == 0 {
		return nil, cursor, nil
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	// Read each value and TTL in the same Redis transaction so a concurrent
	// rebind cannot pair a channel ID with the expiry of a different binding.
	pipe := c.redis.TxPipeline()
	values := make([]*redis.StringCmd, len(keys))
	ttls := make([]*redis.DurationCmd, len(keys))
	for i, key := range keys {
		values[i] = pipe.Get(ctx, key)
		ttls[i] = pipe.PTTL(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, 0, err
	}
	entries := make([]InspectedEntry[V], 0, len(keys))
	for i, key := range keys {
		raw, err := values[i].Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		ttl, err := ttls[i].Result()
		if err != nil {
			return nil, 0, err
		}
		if ttl == -2 {
			continue
		}
		value, err := c.redisCodec.Decode(raw)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, InspectedEntry[V]{Key: key, Value: value, TTL: ttl})
	}
	return entries, cursor, nil
}
