package controller

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// parseChannelTypesFilter is limited to administrative reads. It never grants
// routing eligibility or changes the legacy single-type query semantics.
func parseChannelTypesFilter(c *gin.Context) ([]int, bool) {
	values, present := c.Request.URL.Query()["types"]
	if !present {
		return nil, true
	}
	_, hasType := c.Request.URL.Query()["type"]
	types := make([]int, 0)
	if !hasType && len(values) == 1 {
		for _, value := range strings.Split(values[0], ",") {
			n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
			if err != nil || n <= 0 {
				types = nil
				break
			}
			if !slices.Contains(types, int(n)) {
				types = append(types, int(n))
			}
		}
	}
	if len(types) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams), "error_code": "invalid_channel_types"})
		return nil, false
	}
	return types, true
}

func filterChannelsByTypes(channels []*model.Channel, types []int) []*model.Channel {
	if len(types) == 0 {
		return channels
	}
	filtered := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if slices.Contains(types, channel.Type) {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

// Multi-type tag searches page whole matching tags, just like the list endpoint.
// Legacy searches retain their existing pagination behavior.
func respondChannelTypeTagSearch(c *gin.Context, channels []*model.Channel, typeCounts map[int64]int64) {
	tags := make([]string, 0)
	for _, channel := range channels {
		if channel.Tag != nil && *channel.Tag != "" && !slices.Contains(tags, *channel.Tag) {
			tags = append(tags, *channel.Tag)
		}
	}
	sort.Strings(tags)
	page := common.GetPageQuery(c)
	if page.Page < 1 || page.PageSize < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgInvalidParams), "error_code": "invalid_channel_pagination"})
		return
	}
	// Check the page against the result length before multiplying, so even
	// an out-of-range integer page cannot overflow into a negative index.
	start := len(tags)
	if page.Page-1 <= len(tags)/page.PageSize {
		start = (page.Page - 1) * page.PageSize
	}
	selected := tags[start : start+min(page.PageSize, len(tags)-start)]
	items := make([]*model.Channel, 0)
	for _, channel := range channels {
		if channel.Tag != nil && slices.Contains(selected, *channel.Tag) {
			clearChannelInfo(channel)
			items = append(items, channel)
		}
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": len(tags), "type_counts": typeCounts})
}
