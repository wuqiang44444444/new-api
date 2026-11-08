package controller

import (
	"context"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Image artifacts use their persisted object references, including when the
// original request selected b64_json. The dashboard does not need a plugin or
// another copy of those results.
func projectImageTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, bool) {
	if !model.IsImageTask(task) {
		return nil, false
	}
	if task.Status != model.TaskStatusSuccess || task.ClientDeletedAt != 0 || task.PrivateData.ImageTask == nil {
		return nil, true
	}
	artifacts := make([]relaychannel.TaskArtifact, 0, len(task.PrivateData.ImageTask.Artifacts))
	for _, artifact := range task.PrivateData.ImageTask.Artifacts {
		artifacts = append(artifacts, relaychannel.TaskArtifact{
			Key: path.Base(artifact.ObjectKey), Type: "image", MimeType: artifact.MimeType,
		})
	}
	return artifacts, true
}

func authorizeImageTaskArtifact(c *gin.Context, task *model.Task) bool {
	if !model.IsImageTask(task) {
		return true
	}
	if task.ClientDeletedAt != 0 || (c.GetInt("token_id") > 0 && task.AppID != c.GetInt("token_id")) {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return false
	}
	return true
}

func serveImageTaskArtifact(c *gin.Context, task *model.Task, artifactKey string) bool {
	if !model.IsImageTask(task) {
		return false
	}
	artifact := model.FindImageTaskArtifact(task, artifactKey)
	if artifact == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	ctx, err := service.WithImageObjectStore(ctx)
	if err != nil {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_unavailable", "Artifact is unavailable")
		return true
	}
	exists, err := service.HeadImageObject(ctx, artifact.ObjectKey)
	if err != nil {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_unavailable", "Artifact is unavailable")
		return true
	}
	if !exists {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return true
	}
	if c.Request.Method == http.MethodHead {
		c.Header("Content-Type", artifact.MimeType)
		c.Header("Content-Length", strconv.FormatInt(artifact.Size, 10))
		c.Status(http.StatusOK)
		return true
	}
	url, _, err := service.PresignImageObjectURL(ctx, artifact.ObjectKey)
	if err != nil {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_unavailable", "Artifact is unavailable")
		return true
	}
	c.Redirect(http.StatusFound, url)
	return true
}
