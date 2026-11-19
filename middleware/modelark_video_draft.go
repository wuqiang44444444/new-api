package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func prepareModelArkVideoRequest(body map[string]any, request *dto.ModelArkVideoCreateRequest) (string, error) {
	if raw, present := body["draft"]; present {
		if _, ok := raw.(bool); !ok {
			return "", errors.New("draft must be a boolean")
		}
		if !*request.Draft {
			request.Draft = nil
		}
	}
	for _, item := range request.Content {
		if item.Type != "draft_task" && item.DraftTask == nil {
			continue
		}
		id := strings.TrimSpace(request.DraftSourceID())
		if id == "" || len(id) > 191 || strings.ContainsAny(id, "/\\?#\r\n\t ") || request.Draft != nil {
			return "", errors.New("content must contain exactly one valid draft_task reference")
		}
		if request.Resolution == nil || !modelArkVideoValueAllowed(*request.Resolution, "480p", "720p", "1080p", "4k") {
			return "", errors.New("a valid resolution is required for draft_task")
		}
		for key := range body {
			switch key {
			case "model", "content", "resolution", "draft":
			default:
				return "", errors.New("draft_task cannot be combined with other generation parameters")
			}
		}
		rawItem := body["content"].([]any)[0].(map[string]any)
		if len(rawItem) != 2 {
			return "", errors.New("draft_task cannot be combined with other content")
		}
		if err := rejectUnknownNestedVideoFields(rawItem["draft_task"], "draft_task", "id"); err != nil {
			return "", fmt.Errorf("invalid draft_task: %w", err)
		}
		request.Content[0].DraftTask.ID = id
		request.Resolution = common.GetPointer(strings.TrimSpace(*request.Resolution))
		return "", nil
	}
	// A null or otherwise misplaced draft_task is not a valid ordinary item.
	for _, item := range body["content"].([]any) {
		if _, ok := item.(map[string]any)["draft_task"]; ok {
			return "", errors.New("draft_task requires a dedicated content item")
		}
	}
	return validateModelArkVideoCreateRequest(*request)
}

func rejectSeedanceDraftSource(c *gin.Context) {
	abortModelArkVideo(c, http.StatusBadRequest, "invalid_draft_task", "the requested draft task cannot be used")
}

func pinSeedanceDraftSource(c *gin.Context, request *dto.ModelArkVideoCreateRequest, customerModel string) bool {
	id := request.DraftSourceID()
	if id == "" {
		return true
	}
	task, exists, err := model.GetByTaskIDForApp(c.GetInt("id"), c.GetInt("token_id"), id)
	if err != nil {
		abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "draft task lookup failed")
		return false
	}
	if !exists || !task.IsSeedanceDraftSource() ||
		task.ClientProtocol != model.TaskClientProtocolModelArkV3 || task.ClientDeletedAt != 0 ||
		task.Status != model.TaskStatusSuccess ||
		task.Properties.OriginModelName != customerModel || task.PrivateData.UpstreamTaskID == "" ||
		model.ModelArkDraftMarker(task.Data) != model.ModelArkConfirmedDraft ||
		taskseedance.VideoDraftExpired(task, time.Now()) {
		rejectSeedanceDraftSource(c)
		return false
	}
	constraints := service.GetChannelConstraints(c)
	constraints.AddPin(dto.ChannelPin{
		ChannelId: task.ChannelId,
		Source:    dto.PinSourceOriginTask,
		Rank:      dto.PinRankOriginTask,
		RetryMode: dto.PinRetrySingleAttempt,
	})
	if pin, found, _ := constraints.ResolvedPin(); !found || pin.ChannelId != task.ChannelId {
		rejectSeedanceDraftSource(c)
		return false
	}
	common.SetContextKey(c, constant.ContextKeyOriginTasks, []*model.Task{task})
	return true
}
