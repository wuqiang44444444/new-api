package model

import (
	"github.com/QuantumNous/new-api/common"
)

// 音视频请求证据（一期）：主库索引 + 私有加密对象。
// 索引只描述发生过的事实，正文在独立命名空间的加密对象中；
// 证据不是任务状态、资金或重放许可的第二事实源。
const (
	TaskRequestEvidenceStageNorthReceive     = "north_receive"
	TaskRequestEvidenceStageFulfillment      = "fulfillment"
	TaskRequestEvidenceStageSouthboundSend   = "southbound_send"
	TaskRequestEvidenceStageTransport        = "transport"
	TaskRequestEvidenceStageUpstreamResponse = "upstream_response"
	TaskRequestEvidenceStageClientDelivery   = "client_delivery"
	TaskRequestEvidenceStagePolling          = "polling"

	TaskRequestEvidencePhasePrepared    = "prepared"
	TaskRequestEvidencePhaseResponded   = "responded"
	TaskRequestEvidencePhaseCompleted   = "completed"
	TaskRequestEvidencePhaseTruncated   = "truncated"
	TaskRequestEvidencePhaseFailed      = "failed"
	TaskRequestEvidencePhaseUnavailable = "unavailable"

	TaskRequestEvidenceKindVideoTask       = "video_task"
	TaskRequestEvidenceKindAudioTask       = "audio_task"
	TaskRequestEvidenceKindAudioSpeech     = "audio_speech"
	TaskRequestEvidenceKindAudioTranscribe = "audio_transcription"
	TaskRequestEvidenceKindAudioTranslate  = "audio_translation"
)

type TaskRequestEvidence struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	RequestID   string `json:"request_id" gorm:"type:varchar(64);index"`
	UserID      int    `json:"user_id" gorm:"index"`
	TokenID     int    `json:"token_id"`
	AppID       int    `json:"app_id"`
	TaskID      string `json:"task_id" gorm:"type:varchar(191);index"`
	AttemptID   int64  `json:"attempt_id"`
	Kind        string `json:"kind" gorm:"type:varchar(32);index"`
	ClientModel string `json:"client_model" gorm:"type:varchar(255)"`
	// UpstreamModel/UpstreamRequestID 属于上游诊断事实，仅 Root 查询接口投影，
	// 普通用户与受权支持人员的列表/详情不返回。
	UpstreamModel     string `json:"-" gorm:"type:varchar(255)"`
	UpstreamRequestID string `json:"-" gorm:"type:varchar(128);index"`
	ChannelID         int    `json:"channel_id"`
	ClientProtocol    string `json:"client_protocol" gorm:"type:varchar(32)"`
	UpstreamProtocol  string `json:"-" gorm:"type:varchar(64)"`
	AdapterVersion    string `json:"-" gorm:"type:varchar(128)"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
	UpdatedAt         int64  `json:"updated_at"`
	RetainUntil       int64  `json:"retain_until" gorm:"index"`
	BodyExpired       bool   `json:"body_expired" gorm:"index"`
}

type TaskRequestEvidenceEvent struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	EvidenceId  int64  `json:"evidence_id" gorm:"index:idx_evidence_events_evidence"`
	Seq         int64  `json:"seq"`
	Stage       string `json:"stage" gorm:"type:varchar(32)"`
	AttemptSeq  int    `json:"attempt_seq"`
	Phase       string `json:"phase" gorm:"type:varchar(32)"`
	StatusCode  int    `json:"status_code"`
	Method      string `json:"method" gorm:"type:varchar(16)"`
	Target      string `json:"target" gorm:"type:text"`
	ObjectKey   string `json:"object_key" gorm:"type:varchar(160)"`
	ContentType string `json:"content_type" gorm:"type:varchar(128)"`
	// ByteCount 是观测到的原始字节数，StoredBytes 是实际持久化字节数；
	// 脱敏也可能改变长度；Complete 由采集边界的 EOF、错误和截断事实决定。
	ByteCount   int64  `json:"byte_count"`
	StoredBytes int64  `json:"stored_bytes"`
	Sha256      string `json:"sha256" gorm:"type:char(64)"`
	Complete    bool   `json:"complete"`
	Redacted    bool   `json:"redacted"`
	// Detail 是小型脱敏 JSON（头摘要、错误、归一化说明等），不放大正文。
	Detail    string `json:"detail" gorm:"type:text"`
	CreatedAt int64  `json:"created_at"`
}

type TaskRequestEvidenceAccessLog struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	EvidenceId     int64  `json:"evidence_id" gorm:"index"`
	OperatorUserID int    `json:"operator_user_id"`
	Action         string `json:"action" gorm:"type:varchar(16)"`
	Scope          string `json:"scope" gorm:"type:varchar(16)"`
	CreatedAt      int64  `json:"created_at"`
}

func CreateTaskRequestEvidence(evidence *TaskRequestEvidence) error {
	return DB.Create(evidence).Error
}

func CreateTaskRequestEvidenceEvent(event *TaskRequestEvidenceEvent) error {
	return DB.Create(event).Error
}

func GetTaskRequestEvidenceById(id int64) (*TaskRequestEvidence, bool, error) {
	if id <= 0 {
		return nil, false, nil
	}
	var evidence TaskRequestEvidence
	err := DB.Where("id = ?", id).First(&evidence).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	return &evidence, true, nil
}

func GetTaskRequestEvidenceEvent(evidenceId, eventId int64) (*TaskRequestEvidenceEvent, bool, error) {
	if evidenceId <= 0 || eventId <= 0 {
		return nil, false, nil
	}
	var event TaskRequestEvidenceEvent
	err := DB.Where("id = ? AND evidence_id = ?", eventId, evidenceId).First(&event).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	return &event, true, nil
}

// TaskRequestEvidenceQueryParams 约束证据检索：索引字段查询，永不返回正文。
type TaskRequestEvidenceQueryParams struct {
	RequestID         string
	UpstreamRequestID string
	TaskID            string
	Kind              string
	StartIdx          int
	Num               int
}

func QueryTaskRequestEvidence(params TaskRequestEvidenceQueryParams) ([]*TaskRequestEvidence, int64) {
	query := DB.Model(&TaskRequestEvidence{})
	if params.RequestID != "" {
		query = query.Where("request_id = ?", params.RequestID)
	}
	if params.UpstreamRequestID != "" {
		query = query.Where("upstream_request_id = ?", params.UpstreamRequestID)
	}
	if params.TaskID != "" {
		query = query.Where("task_id = ?", params.TaskID)
	}
	if params.Kind != "" {
		query = query.Where("kind = ?", params.Kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0
	}
	if params.Num <= 0 || params.Num > 100 {
		params.Num = 20
	}
	var items []*TaskRequestEvidence
	if err := query.Order("id desc").Limit(params.Num).Offset(params.StartIdx).Find(&items).Error; err != nil {
		return nil, 0
	}
	return items, total
}

func ListTaskRequestEvidenceEvents(evidenceId int64) ([]*TaskRequestEvidenceEvent, error) {
	var events []*TaskRequestEvidenceEvent
	err := DB.Where("evidence_id = ?", evidenceId).Order("id").Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func RecordTaskRequestEvidenceAccess(evidenceId, operatorUserID int64, action, scope string) {
	entry := &TaskRequestEvidenceAccessLog{
		EvidenceId:     evidenceId,
		OperatorUserID: int(operatorUserID),
		Action:         action,
		Scope:          scope,
		CreatedAt:      common.GetTimestamp(),
	}
	if err := DB.Create(entry).Error; err != nil {
		common.SysError("record evidence access failed: " + err.Error())
	}
}

// AttachTaskRequestEvidenceTask 在任务行原子落库后回填关联。
// 失败只记录告警，不回滚任务或资金事实。
func AttachTaskRequestEvidenceTask(evidenceId int64, taskID string, attemptID int64) {
	if evidenceId <= 0 || taskID == "" {
		return
	}
	updates := map[string]any{"task_id": taskID, "updated_at": common.GetTimestamp()}
	if attemptID != 0 {
		updates["attempt_id"] = attemptID
	}
	if err := DB.Model(&TaskRequestEvidence{}).Where("id = ?", evidenceId).Updates(updates).Error; err != nil {
		common.SysError("attach task request evidence task failed: " + err.Error())
	}
}

func FindTaskRequestEvidenceByTaskID(taskID string) (*TaskRequestEvidence, bool, error) {
	if taskID == "" {
		return nil, false, nil
	}
	var evidence TaskRequestEvidence
	err := DB.Where("task_id = ?", taskID).Order("id").First(&evidence).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	return &evidence, true, nil
}

// UpdateTaskRequestEvidenceFacts 增量更新索引事实列；零值字段跳过。
func UpdateTaskRequestEvidenceFacts(evidenceId int64, updates map[string]any) {
	if evidenceId <= 0 || len(updates) == 0 {
		return
	}
	updates["updated_at"] = common.GetTimestamp()
	if err := DB.Model(&TaskRequestEvidence{}).Where("id = ?", evidenceId).Updates(updates).Error; err != nil {
		common.SysError("update evidence facts failed: " + err.Error())
	}
}

func UpsertTaskRequestEvidenceUpstreamRequestID(evidenceId int64, upstreamRequestID string) {
	if evidenceId <= 0 || upstreamRequestID == "" {
		return
	}
	err := DB.Model(&TaskRequestEvidence{}).
		Where("id = ? AND (upstream_request_id = '' OR upstream_request_id IS NULL)", evidenceId).
		Updates(map[string]any{"upstream_request_id": upstreamRequestID, "updated_at": common.GetTimestamp()}).Error
	if err != nil {
		common.SysError("attach evidence upstream request id failed: " + err.Error())
	}
}

// TaskRequestEvidenceExpiredBodies 返回需要清理正文（保留索引）的行。
func TaskRequestEvidenceExpiredBodies(now int64, limit int) ([]*TaskRequestEvidence, error) {
	if limit <= 0 {
		limit = 100
	}
	var items []*TaskRequestEvidence
	err := DB.Where("body_expired = ? AND retain_until > 0 AND retain_until < ?", false, now).
		Order("id").Limit(limit).Find(&items).Error
	return items, err
}

func MarkTaskRequestEvidenceBodyExpired(evidenceId int64) error {
	return DB.Model(&TaskRequestEvidence{}).Where("id = ?", evidenceId).
		Updates(map[string]any{"body_expired": true, "updated_at": common.GetTimestamp()}).Error
}

// FinishTaskRequestEvidenceEvent updates the reserved event, including zero values.
func FinishTaskRequestEvidenceEvent(event *TaskRequestEvidenceEvent) error {
	return DB.Model(&TaskRequestEvidenceEvent{}).Where("id = ? AND evidence_id = ?", event.Id, event.EvidenceId).Select("*").Updates(event).Error
}
