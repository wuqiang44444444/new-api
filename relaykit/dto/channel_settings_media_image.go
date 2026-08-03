package dto

import "strings"

func (c *AdvancedCustomConfig) SupportsPersistentMediaImageTask(requestPath string, model string) bool {
	if c == nil {
		return false
	}
	route, ok := c.MatchPathForModel(requestPath, model)
	return ok && strings.TrimSpace(route.Converter) == AdvancedCustomConverterMediaTaskImageBlocking
}
