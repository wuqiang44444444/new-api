package model

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

// Only a short-lived reporting projection, never a settlement/export source.
// Serialized entries are immutable and capped at 8 MiB each, four entries total.
var upstreamPageCache = hot.NewHotCache[string, []byte](hot.LRU, 4).Build()
var upstreamPageLoads singleflight.Group

func cachedUpstreamSummary(ctx context.Context, start, end, period int64) (ProviderURLSummary, error) {
	var summary ProviderURLSummary
	metadata, err := upstreamSummaryMetadata(ctx, period)
	if err != nil {
		return summary, err
	}
	key := fmt.Sprintf("%p:%p:%d:%d:%d:%x", DB, LOG_DB, start, end, period, common.Sha256Raw(metadata))
	if encoded, ok, err := upstreamPageCache.Get(key); err == nil && ok {
		err = common.Unmarshal(encoded, &summary)
		return summary, err
	}
	// Singleflight work has its own bounded lifetime: one closed browser request
	// must not cancel the shared calculation required by the other page requests.
	result := upstreamPageLoads.DoChan(key, func() (any, error) {
		if encoded, ok, err := upstreamPageCache.Get(key); err == nil && ok {
			return encoded, nil
		}
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		data, err := getProviderBillingURLSummary(work, start, end, period, "", 0)
		if err != nil {
			return nil, err
		}
		encoded, err := common.Marshal(data)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= 8*1024*1024 {
			upstreamPageCache.SetWithTTL(key, encoded, 30*time.Second)
		}
		return encoded, nil
	})
	select {
	case <-ctx.Done():
		return summary, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return summary, value.Err
		}
		err = common.Unmarshal(value.Val.([]byte), &summary)
		return summary, err
	}
}
