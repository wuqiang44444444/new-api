package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetUpstreamBalanceConnections(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channels, err := model.GetUpstreamBalanceChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to load upstream connections"})
		return
	}
	groups, err := model.GetUpstreamBalanceURLGroups(channels)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to load upstream connections"})
		return
	}
	// Group before deduplicating keys so unidentifiable URLs retain the same
	// per-channel isolation used by reconciliation, even with equal credentials.
	groupChannels := make(map[string][]*model.Channel)
	groupOrder := make([]model.UpstreamBalanceURLGroup, 0)
	for _, channel := range channels {
		group := groups[channel.Id]
		if _, exists := groupChannels[group.URLKey]; !exists {
			groupOrder = append(groupOrder, group)
		}
		groupChannels[group.URLKey] = append(groupChannels[group.URLKey], channel)
	}
	rows := make([]service.UpstreamBalanceConnection, 0)
	for _, group := range groupOrder {
		for _, row := range service.UpstreamBalanceInventory(groupChannels[group.URLKey]) {
			row.URLKey, row.GroupName = group.URLKey, group.Name
			rows = append(rows, row)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func GetUpstreamBalance(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, idErr := strconv.Atoi(c.Param("id"))
	keyIndex, keyErr := strconv.Atoi(c.Param("key_index"))
	if idErr != nil || id <= 0 || keyErr != nil || keyIndex < 0 || len(c.Query("reference")) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid upstream connection"})
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Upstream connection is unavailable"})
		return
	}
	result := service.QueryUpstreamBalance(c.Request.Context(), channel, keyIndex, c.Query("reference"))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
