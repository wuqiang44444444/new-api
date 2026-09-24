package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	taskminimax "github.com/QuantumNous/new-api/relay/channel/task/minimax"
	"github.com/gin-gonic/gin"
)

// MiniMax persists trusted success but never the temporary JD media URL. Its
// typed task and upstream ID identify the video; the frozen adapter resolves
// the current content source only when it is downloaded.
func projectMiniMaxTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, bool) {
	if task == nil || task.Platform != taskminimax.Platform {
		return nil, false
	}
	if task.Status != model.TaskStatusSuccess || task.ClientDeletedAt != 0 ||
		strings.TrimSpace(task.PrivateData.UpstreamTaskID) == "" {
		return nil, true
	}
	return []relaychannel.TaskArtifact{{Key: "video", Type: "video", MimeType: "video/mp4"}}, true
}

// User/session and signed-artifact authorization remains with the caller.
// API keys must additionally match the typed task's frozen application.
func authorizeMiniMaxTaskArtifact(c *gin.Context, task *model.Task) bool {
	if task == nil || task.Platform != taskminimax.Platform {
		return true
	}
	appID := c.GetInt("token_id")
	if task.ClientDeletedAt != 0 || (appID > 0 && task.AppID != appID) {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return false
	}
	return true
}

func serveMiniMaxTaskArtifact(c *gin.Context, task *model.Task, artifactKey string) bool {
	artifacts, handled := projectMiniMaxTaskArtifacts(task)
	if !handled {
		return false
	}
	if len(artifacts) == 0 || artifactKey != artifacts[0].Key || c.Query("part") != "" {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return true
	}
	if !model.TaskUsesFrozenVideoConnection(task) {
		writeTaskArtifactError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
		return true
	}
	return proxyLinkVideoTaskContent(c, task)
}
