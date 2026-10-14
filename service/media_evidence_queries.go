package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Query batches retain the actual exchange only on eligible task indexes. A
// batch itself cannot open another window or authorize text-task collection.
func finishMediaEvidenceQueries(capture *ImageErrorEvidence, indexes []*model.TaskRequestEvidence, status int, headers http.Header) {
	for _, index := range indexes {
		bound := newAPIRequestEvidence(*index)
		if bound == nil {
			continue
		}
		bound.records = append(bound.records, capture.records...)
		bound.secrets = append(bound.secrets, capture.secrets...)
		bound.deliveryStatsOnly = capture.deliveryStatsOnly
		bound.FinishClient(status, headers)
	}
	capture.Abandon()
}

func finishMidjourneyRequestEvidence(c *gin.Context, capture *ImageErrorEvidence, request *imageEvidenceExchange, status int, headers http.Header) bool {
	operation := midjourneyEvidenceOperation(c.Request.Method, c.Request.URL.Path)
	if operation == "" {
		return false
	}
	capture.Index.ClientProtocol = "midjourney"
	userID, appID := capture.Index.UserID, capture.Index.AppID
	if operation == "create" {
		if userID <= 0 || appID <= 0 || status >= 400 {
			return false // Rejected media creates keep their independent request evidence.
		}
		var response struct {
			Result string `json:"result"`
		}
		for _, record := range capture.records {
			if record.Stage == model.TaskRequestEvidenceStageClientDelivery && common.Unmarshal(record.Body.Bytes(), &response) == nil {
				break
			}
		}
		if response.Result == "" {
			return false
		}
		index, err := model.GetMidjourneyMediaEvidence(capture.Index, response.Result, common.GetTimestamp(), EvidenceRetentionSeconds())
		if err == nil {
			capture.Index = *index
		} else if err != model.ErrTaskRequestEvidenceBodyClosed && err != gorm.ErrRecordNotFound {
			common.SysError("evidence MJ creation binding unavailable")
		}
		if err == model.ErrTaskRequestEvidenceBodyClosed {
			// An existing closed task must not acquire a fresh creation index.
			capture.Abandon()
			return true
		}
		return false
	}
	var ids []string
	if c.Request.Method == http.MethodGet {
		ids = []string{c.Param("id")}
	} else {
		var query struct {
			IDs []string `json:"ids"`
		}
		if common.Unmarshal(request.Body.Bytes(), &query) == nil {
			ids = query.IDs
		}
	}
	var indexes []*model.TaskRequestEvidence
	seen := make(map[string]bool)
	for _, id := range ids {
		if id == "" || seen[id] || userID <= 0 || appID <= 0 {
			continue
		}
		seen[id] = true
		seed := capture.Index
		seed.CreatedAt = 0
		index, err := model.GetMidjourneyMediaEvidence(seed, id, common.GetTimestamp(), EvidenceRetentionSeconds())
		if err == nil {
			indexes = append(indexes, index)
		} else if err == model.ErrTaskRequestEvidenceCreationTime {
			common.SysError("evidence MJ creation anchor unavailable; capture skipped")
		} else if err != model.ErrTaskRequestEvidenceBodyClosed && err != gorm.ErrRecordNotFound {
			common.SysError("evidence MJ query index unavailable")
		}
	}
	finishMediaEvidenceQueries(capture, indexes, status, headers)
	return true
}

func finishSunoBatchRequestEvidence(c *gin.Context, capture *ImageErrorEvidence, request *imageEvidenceExchange, status int, headers http.Header) bool {
	if !strings.HasPrefix(c.Request.URL.Path, "/suno/") || capture.Index.Kind != "task_query" {
		return false
	}
	var query struct {
		IDs []string `json:"ids"`
	}
	if c.Request.Method == http.MethodPost {
		_ = common.Unmarshal(request.Body.Bytes(), &query)
	}
	var indexes []*model.TaskRequestEvidence
	seen := make(map[string]bool)
	for _, id := range query.IDs {
		if id == "" || seen[id] || capture.Index.UserID <= 0 || capture.Index.AppID <= 0 {
			continue
		}
		seen[id] = true
		task, exists, err := model.GetByTaskIDForApp(capture.Index.UserID, capture.Index.AppID, id)
		if err != nil || !exists {
			continue
		}
		index, err := taskMediaEvidenceIndex(task)
		if err == nil && index != nil {
			indexes = append(indexes, index)
		}
	}
	finishMediaEvidenceQueries(capture, indexes, status, headers)
	return true
}
