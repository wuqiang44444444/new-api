package cachex

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// CompareAndSwapWithTTL stores a binding if it still has the value seen by the
// caller. A nil expected value requires the key to be absent.
func (c *HybridCache[V]) CompareAndSwapWithTTL(key string, expected *V, value V, ttl time.Duration) (bool, error) {
	full := c.ns.FullKey(key)
	if full == "" || ttl <= 0 {
		return false, errors.New("cache compare-and-swap requires a key and positive TTL")
	}
	if !c.redisOn() {
		lock := c.writeLock(full)
		lock.Lock()
		defer lock.Unlock()
		cache := c.memCache()
		current, found, err := cache.Get(full)
		if err != nil || found != (expected != nil) {
			return false, err
		}
		if found && !reflect.DeepEqual(current, *expected) {
			return false, nil
		}
		cache.SetWithTTL(full, value, ttl)
		return true, nil
	}

	encoded, err := c.redisCodec.Encode(value)
	if err != nil {
		return false, err
	}
	var previous string
	if expected != nil {
		previous, err = c.redisCodec.Encode(*expected)
		if err != nil {
			return false, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.redisOpTimeout)
	defer cancel()
	result, err := c.redis.Eval(ctx, compareAndSwapTTL, []string{full}, strconv.FormatBool(expected != nil), previous, encoded, max(ttl.Milliseconds(), 1)).Int()
	return result == 1, err
}

var cacheWriteLocks [256]sync.Mutex

func (c *HybridCache[V]) writeLock(key string) *sync.Mutex {
	var hash uint8
	for i := range len(key) {
		hash = hash*31 + key[i]
	}
	return &cacheWriteLocks[hash]
}

const compareAndSwapTTL = `
local current = redis.call('GET', KEYS[1])
if (ARGV[1] == 'true' and current ~= ARGV[2]) or (ARGV[1] == 'false' and current ~= false) then
  return 0
end
redis.call('SET', KEYS[1], ARGV[3], 'PX', ARGV[4])
return 1
`
