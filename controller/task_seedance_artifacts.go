package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/gin-gonic/gin"
)

func isSeedanceArtifactTask(task *model.Task) bool {
	return task != nil && task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
}

// Seedance has a code-registered adapter, even when its execution snapshot
// includes plugin provenance. Artifact identity comes from the persisted result.
func projectSeedanceTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, bool) {
	if !isSeedanceArtifactTask(task) {
		return nil, false
	}
	resultURL := strings.TrimSpace(task.PrivateData.ResultURL)
	if task.Status != model.TaskStatusSuccess || task.ClientDeletedAt != 0 ||
		resultURL == "" || isTaskMediaFallbackLoop(resultURL, task.TaskID) {
		return nil, true
	}
	return []relaychannel.TaskArtifact{{Key: "video", Type: "video", MimeType: "video/mp4"}}, true
}

// User/session or signed-artifact authorization has already run at the caller.
// API keys additionally retain the Link task's app boundary.
func authorizeSeedanceTaskArtifact(c *gin.Context, task *model.Task) bool {
	if !isSeedanceArtifactTask(task) {
		return true
	}
	appID := c.GetInt("token_id")
	if task.ClientDeletedAt != 0 || (appID > 0 && task.AppID != appID) {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return false
	}
	return true
}

func serveSeedanceTaskArtifact(c *gin.Context, task *model.Task, artifactKey string) bool {
	artifacts, handled := projectSeedanceTaskArtifacts(task)
	if !handled {
		return false
	}
	// The video capability must not also grant access to a different content part.
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
