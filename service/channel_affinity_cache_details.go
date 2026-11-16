package service

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type ChannelAffinityCacheEntry struct {
	Fingerprint string `json:"fingerprint"`
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	TTLSeconds  *int64 `json:"ttl_seconds"`
}

type ChannelAffinityCachePage struct {
	Entries    []ChannelAffinityCacheEntry `json:"entries"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

func GetChannelAffinityCacheDetails(ctx context.Context, layer string, cursor uint64) (ChannelAffinityCachePage, error) {
	cache := channelAffinityCacheForLayer(layer)
	if cache == nil {
		return ChannelAffinityCachePage{}, common.NewMessage("Invalid parameters")
	}
	bindings, nextCursor, err := cache.Inspect(ctx, cursor)
	if err != nil {
		return ChannelAffinityCachePage{}, err
	}
	ids := make([]int, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.Value)
	}
	names, err := model.ReadDailyReportChannelNames(ctx, ids)
	if err != nil {
		return ChannelAffinityCachePage{}, err
	}
	page := ChannelAffinityCachePage{Entries: make([]ChannelAffinityCacheEntry, 0, len(bindings))}
	for _, binding := range bindings {
		entry := ChannelAffinityCacheEntry{
			Fingerprint: common.HmacSha256("affinity-inspection:"+binding.Key, common.SessionSecret)[:16],
			ChannelID:   binding.Value,
			ChannelName: names[binding.Value],
		}
		if binding.TTL >= 0 {
			seconds := int64(binding.TTL / time.Second)
			if binding.TTL%time.Second != 0 {
				seconds++
			}
			entry.TTLSeconds = &seconds
		}
		page.Entries = append(page.Entries, entry)
	}
	if nextCursor != 0 {
		page.NextCursor = strconv.FormatUint(nextCursor, 10)
	}
	return page, nil
}
