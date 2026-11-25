package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ListArkCacheShadowHours(c *gin.Context) {
	var query struct {
		ChannelID int    `form:"channel_id"`
		UserID    int    `form:"user_id"`
		AppID     int    `form:"app_id"`
		Model     string `form:"model"`
		Start     int64  `form:"hour_start"`
		End       int64  `form:"hour_end"`
	}
	if c.ShouldBindQuery(&query) != nil {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_shadow_window")
		return
	}
	hours, err := model.ArkCacheShadowHours(c.Request.Context(), query.ChannelID, query.UserID, query.AppID, query.Model, query.Start, query.End)
	switch {
	case errors.Is(err, model.ErrArkCacheConflict):
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_shadow_scope")
	case err != nil:
		arkCacheStateError(c, err)
	default:
		c.JSON(http.StatusOK, gin.H{"meter": "candidate", "hours": hours})
	}
}
