package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
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
	// CleanupRequestedAt 是持久清理申请标记：手动提前清理受理后立即关闭该
	// 索引的读取与追加，对象全部删除成功后才写 BodyExpired 完成事实。
	// 它只是证据清理状态，不是任务或资金状态机。
	CleanupRequestedAt int64 `json:"cleanup_requested_at" gorm:"index;default:0"`
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
	CreatedAt      int64  `json:"created_at" gorm:"index"`
}

func CreateTaskRequestEvidence(evidence *TaskRequestEvidence) error {
	return DB.Create(evidence).Error
}

func CreateTaskRequestEvidenceEvent(event *TaskRequestEvidenceEvent, retention ...int64) error {
	seconds := EvidenceIndexRetentionSeconds
	if len(retention) > 0 {
		seconds = retention[0]
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := writableEvidenceIndex(tx, event.EvidenceId, common.GetTimestamp(), seconds); err != nil {
			return err
		}
		return tx.Create(event).Error
	})
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

func QueryTaskRequestEvidence(params TaskRequestEvidenceQueryParams) ([]*TaskRequestEvidence, int64, error) {
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
		return nil, 0, err
	}
	if params.Num <= 0 || params.Num > 100 {
		params.Num = 20
	}
	var items []*TaskRequestEvidence
	if err := query.Order("id desc").Limit(params.Num).Offset(params.StartIdx).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
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
func AttachTaskRequestEvidenceTask(evidenceId int64, taskID string, attemptID int64, userID, appID int) {
	if evidenceId <= 0 || taskID == "" {
		return
	}
	updates := map[string]any{"task_id": taskID, "user_id": userID, "app_id": appID, "updated_at": common.GetTimestamp()}
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

func SetTaskRequestEvidenceUpstreamRequestID(evidenceId int64, upstreamRequestID string) {
	if evidenceId <= 0 {
		return
	}
	err := DB.Model(&TaskRequestEvidence{}).
		Where("id = ?", evidenceId).
		Updates(map[string]any{"upstream_request_id": upstreamRequestID, "updated_at": common.GetTimestamp()}).Error
	if err != nil {
		common.SysError("attach evidence upstream request id failed: " + err.Error())
	}
}

// TaskRequestEvidenceExpiredBodies 返回需要清理正文（保留索引）的行。
// 有效到期时间取“创建时间 + 当前保留秒数”与原有非零更早期限中的较早值；
// 历史 retain_until=0（永久保留）同样受当前上限约束，不要求一次性回填。
// targetTimestamp>0 为手动截止：严格要求 created_at < target，等于截止点
// 保留。自动运行另选到期或已申请的行，提前清理不会被七天筛选漏掉。
// afterID 是稳定游标：删除失败未标记的行不会在同一轮内重复出现，也不会
// 饿死后面的记录。条件仅用整数比较，兼容 SQLite/MySQL/PostgreSQL。
func TaskRequestEvidenceExpiredBodies(now, retentionSeconds, targetTimestamp, afterID int64, limit int) ([]*TaskRequestEvidence, error) {
	if limit <= 0 {
		limit = 100
	}
	if retentionSeconds < 86400 {
		retentionSeconds = 7 * 86400
	}
	var items []*TaskRequestEvidence
	query := taskRequestEvidenceCleanupCandidates(now, retentionSeconds, targetTimestamp).Where("id > ?", afterID)
	err := query.Order("id").Limit(limit).Find(&items).Error
	return items, err
}

func taskRequestEvidenceCleanupCandidates(now, retentionSeconds, targetTimestamp int64) *gorm.DB {
	query := DB.Model(&TaskRequestEvidence{}).Where("body_expired = ?", false)
	if targetTimestamp > 0 {
		query = query.Where("created_at < ?", targetTimestamp)
	} else {
		query = query.Where("cleanup_requested_at > 0 OR created_at < ? OR (retain_until > 0 AND retain_until < ?)", now-retentionSeconds, now)
	}
	return query
}

// RequestTaskRequestEvidenceCleanup 登记持久清理申请：申请后立即关闭读取与
// 追加，重复申请不改变既有申请时间。失败由调用方按可重试处理。
func RequestTaskRequestEvidenceCleanup(evidenceId, requestedAt int64) error {
	result := DB.Model(&TaskRequestEvidence{}).
		Where("id = ? AND (cleanup_requested_at = 0 OR cleanup_requested_at IS NULL)", evidenceId).
		Updates(map[string]any{"cleanup_requested_at": requestedAt, "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	index, exists, err := GetTaskRequestEvidenceById(evidenceId)
	if err != nil {
		return err
	}
	if !exists || index.CleanupRequestedAt <= 0 {
		return ErrTaskRequestEvidenceBodyClosed
	}
	return nil
}

// TaskRequestEvidenceBodyClosedForAppend 核对一条证据索引是否已拒绝新增正文：
// 清理已标记，或按当前保留政策已到期。此处是预检；实际文件提交必须经过
// CommitTaskRequestEvidenceObject 的互斥复核，不能依靠预检排除竞争。
func TaskRequestEvidenceBodyClosedForAppend(evidenceId, now, retentionSeconds int64) (bool, error) {
	if evidenceId <= 0 {
		return false, nil
	}
	if retentionSeconds < 86400 {
		retentionSeconds = 7 * 86400
	}
	var evidence TaskRequestEvidence
	err := DB.Select("body_expired", "cleanup_requested_at", "created_at", "retain_until").
		Where("id = ?", evidenceId).First(&evidence).Error
	if err != nil {
		exist, existErr := RecordExist(err)
		if existErr != nil {
			return false, existErr
		}
		if !exist {
			// A deleted index is closed; skip observation without recreating it.
			return true, nil
		}
	}
	if evidence.BodyExpired || evidence.CleanupRequestedAt > 0 {
		return true, nil
	}
	deadline := evidence.CreatedAt + retentionSeconds
	if evidence.RetainUntil > 0 && evidence.RetainUntil < deadline {
		deadline = evidence.RetainUntil
	}
	return deadline < now, nil
}

func MarkTaskRequestEvidenceBodyExpired(evidenceId int64) error {
	return DB.Model(&TaskRequestEvidence{}).Where("id = ?", evidenceId).
		Updates(map[string]any{"body_expired": true, "updated_at": common.GetTimestamp()}).Error
}

// FinishTaskRequestEvidenceEvent updates the reserved event, including zero values.
func FinishTaskRequestEvidenceEvent(event *TaskRequestEvidenceEvent) error {
	result := DB.Model(&TaskRequestEvidenceEvent{}).Where("id = ? AND evidence_id = ?", event.Id, event.EvidenceId).Select("*").Updates(event)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskRequestEvidenceBodyClosed
	}
	return nil
}
