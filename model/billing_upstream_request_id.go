package model

import (
	"context"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const TaskCreateRequestIDLogKey = "task_create_request_id"
const taskBillingDeliveryRequestPrefix = "task-billing:"

// request_id still indexes the original log ID, including delivery keys.
// A task candidate must be checked against its actual gateway request ID.
func upstreamRequestIDCandidates(query *gorm.DB, requestID string) *gorm.DB {
	if requestID == "" {
		return query
	}
	return query.Where("(request_id = ? OR request_id LIKE ?)", requestID, taskBillingDeliveryRequestPrefix+"%")
}

func upstreamRequestIDFromLog(log Log) string {
	if !strings.HasPrefix(log.RequestId, taskBillingDeliveryRequestPrefix) {
		return log.RequestId
	}
	var other struct {
		Admin struct {
			RequestID string `json:"task_create_request_id"`
		} `json:"admin_info"`
	}
	if common.UnmarshalJsonStr(log.Other, &other) == nil {
		if requestID := upstreamTaskCreateRequestID(other.Admin.RequestID); requestID != "" {
			return requestID
		}
	}
	// Historical delivery logs did not include the original request ID.
	return log.RequestId
}

func upstreamTaskCreateRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 64 || strings.HasPrefix(requestID, taskBillingDeliveryRequestPrefix) {
		return ""
	}
	return requestID
}

// Old reports and delivery logs can recover the immutable creation request
// only from the exact task row and billing identity. A delivery key itself
// must never leave the detail projection as a gateway request ID.
func resolveUpstreamDetailRequestIDs(ctx context.Context, items []UpstreamBillingDetailItem) error {
	var ids []int64
	pending := make(map[int64][]int)
	for i := range items {
		item := &items[i]
		key, internal := strings.CutPrefix(item.RequestId, taskBillingDeliveryRequestPrefix)
		if !internal {
			continue
		}
		item.RequestId = ""
		rowID, event, valid := strings.Cut(key, ":")
		if !valid || item.requestUserID <= 0 || item.ChannelId <= 0 {
			continue
		}
		switch event {
		case "create", "adjustment", "refund", "customer_refund", "complete":
		default:
			continue
		}
		id, err := strconv.ParseInt(rowID, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != rowID {
			continue
		}
		if _, found := pending[id]; !found {
			ids = append(ids, id)
		}
		pending[id] = append(pending[id], i)
	}
	for start := 0; start < len(ids); start += trustedTaskSecondsBatch {
		var tasks []Task
		if err := usageReportMainDB(ctx).WithContext(ctx).
			Select("id", "user_id", "channel_id", "task_id", "private_data").
			Where("id IN ?", ids[start:min(start+trustedTaskSecondsBatch, len(ids))]).Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if task.PrivateData.Execution == nil {
				continue
			}
			requestID := upstreamTaskCreateRequestID(task.PrivateData.Execution.RequestID)
			for _, i := range pending[task.ID] {
				item := &items[i]
				if task.UserId == item.requestUserID && task.ChannelId == item.ChannelId &&
					(item.PlatformTaskId == "" || task.TaskID == item.PlatformTaskId) {
					item.RequestId = requestID
				}
			}
		}
	}
	return nil
}
