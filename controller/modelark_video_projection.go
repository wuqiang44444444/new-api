package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func projectModelArkVideoTask(c *gin.Context, task *model.Task) *dto.ModelArkVideoTask {
	projected := task.ToModelArkVideoTask()
	if projected.Content == nil {
		return projected
	}
	baseURL := incomingVideoBaseURL(c)
	if baseURL == "" {
		return projected
	}
	if strings.HasPrefix(projected.Content.VideoURL, "/") {
		projected.Content.VideoURL = baseURL + projected.Content.VideoURL
	}
	if strings.HasPrefix(projected.Content.LastFrameURL, "/") {
		projected.Content.LastFrameURL = baseURL + projected.Content.LastFrameURL
	}
	return projected
}
