package dto

import "strings"

// SupportsPersistentMediaImageTask reports whether the configured Advanced
// Custom route owns the durable media-image task lifecycle for this request.
// Keeping this predicate beside the route contract lets relay preparation and
// idempotency scope use exactly the same boundary.
func (c *AdvancedCustomConfig) SupportsPersistentMediaImageTask(requestPath string, model string) bool {
	if c == nil {
		return false
	}
	route, ok := c.MatchPathForModel(requestPath, model)
	return ok && strings.TrimSpace(route.Converter) == AdvancedCustomConverterMediaTaskImageBlocking
}
