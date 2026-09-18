package model

import (
	"errors"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
)

// ErrorEventModule 取值与 clienterrlog.persistedModules 保持一致；
// 错误事件表只保存 token 网关 API 调用（relay/asset）的已鉴权最终 4xx/5xx。
const (
	ErrorEventModuleRelay = "relay"
	ErrorEventModuleAsset = "asset"
)

// ErrorEvent 是「API 调用错误」独立事件表（docs/80-dev/2026-09-17-全量错误日志独立菜单分析与方案.md）：
//   - 与原生 type=5 错误日志互不影响，不做双计；页面（api/old_api/web）流量不进入本表。
//   - 不保存查询参数、Cookie、凭据、源 URL、签名参数或媒体二进制；
//     Detail.http_exchange 只存 clienterrlog 生成的有界脱敏 JSON 正文快照。
//   - 独立于用量日志的清理与 TTL 策略（与 audit_logs 同先例）。
//   - Detail 是白名单诊断 map 的预编码 JSON 文本；使用普通字符串列可避免
//     ClickHouse GORM 插入回调的 Valuer 问题（见 audit_log.go 的 embedded workaround）。
type ErrorEvent struct {
	Id                int    `json:"id" gorm:"index:idx_error_created_at_id,priority:2"`
	CreatedAt         int64  `json:"created_at" gorm:"type:bigint;index:idx_error_created_at_id,priority:1;index;index:idx_error_status_created,priority:2"`
	Module            string `json:"module" gorm:"type:varchar(16);index"`
	Method            string `json:"method" gorm:"type:varchar(16)"`
	Route             string `json:"route" gorm:"type:varchar(255)"`
	Status            int    `json:"status" gorm:"index:idx_error_status_created,priority:1"`
	UserId            int    `json:"user_id" gorm:"index"`
	Username          string `json:"username" gorm:"type:varchar(64);index;default:''"`
	TokenName         string `json:"token_name" gorm:"type:varchar(64);index;default:''"`
	ModelName         string `json:"model_name" gorm:"type:varchar(128);index;default:''"`
	ChannelId         int    `json:"channel_id" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	EventType         string `json:"event_type" gorm:"type:varchar(32);index;default:''"`
	TaskId            string `json:"task_id" gorm:"type:varchar(64);index;default:''"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	UpstreamRequestId string `json:"upstream_request_id" gorm:"type:varchar(128);index;default:''"`
	Stage             string `json:"stage" gorm:"type:varchar(64)"`
	Reason            string `json:"reason" gorm:"type:varchar(64);index"`
	PublicCode        string `json:"public_code" gorm:"type:varchar(64)"`
	Protocol          string `json:"protocol" gorm:"type:varchar(64)"`
	ElapsedMs         int64  `json:"elapsed_ms" gorm:"type:bigint"`
	Detail            string `json:"detail" gorm:"type:text"`
}

type ErrorEventFilter struct {
	Module            string
	EventType         string
	Username          string
	TokenName         string
	ModelName         string
	RequestId         string
	UpstreamRequestId string
	Reason            string
	TaskId            string
	Status            *int // 按列表状态筛选：渠道测试用上游状态，其余用客户状态；nil=全部，0=未记录
	UserId            int
	ChannelId         int
	StartTimestamp    int64
	EndTimestamp      int64
}

func init() {
	clienterrlog.SetEventPersister(persistErrorEvent)
}

// persistErrorEvent 把 clienterrlog 事件转换为受控行并写入日志库。失败必须返回
// 错误，由 sink 计入 persist_failed，不得静默视为成功。
func persistErrorEvent(e clienterrlog.Event) error {
	if LOG_DB == nil {
		return errors.New("log database unavailable")
	}
	eventType := e.EventType
	if eventType == "" {
		eventType = clienterrlog.EventAPIError
	}
	row := &ErrorEvent{
		CreatedAt:         e.At.Unix(),
		Module:            e.Module,
		EventType:         eventType,
		TaskId:            e.TaskID,
		Method:            e.Method,
		Route:             e.Route,
		Status:            e.Status,
		UserId:            e.UserID,
		Username:          e.Username,
		TokenName:         e.TokenName,
		ModelName:         e.Model,
		ChannelId:         e.ChannelID,
		RequestId:         e.RequestID,
		UpstreamRequestId: e.UpstreamRequestID,
		Stage:             e.Stage,
		Reason:            e.Reason,
		PublicCode:        e.PublicCode,
		Protocol:          e.Protocol,
		ElapsedMs:         e.ElapsedMs,
	}
	if e.HTTPExchange != nil {
		encoded, err := common.Marshal(e.HTTPExchange)
		if err != nil {
			return err
		}
		if e.Detail == nil {
			e.Detail = map[string]string{}
		}
		e.Detail[clienterrlog.HTTPExchangeDetailKey] = string(encoded)
	}
	if len(e.Detail) > 0 {
		encoded, err := common.Marshal(e.Detail)
		if err != nil {
			return err
		}
		row.Detail = string(encoded)
	}
	return LOG_DB.Create(row).Error
}

func ValidErrorEventModule(module string) bool {
	return module == "" || module == ErrorEventModuleRelay || module == ErrorEventModuleAsset
}

// MigrateErrorEvents 支持独立配置的 ClickHouse 日志库；与 audit_logs 一致，
// 刻意不带 TTL、不接入用量日志清理。ORDER BY 与查询排序键保持一致。
func MigrateErrorEvents() error {
	if LOG_DB == nil {
		return errors.New("log database unavailable")
	}
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		if err := LOG_DB.AutoMigrate(&ErrorEvent{}); err != nil {
			return err
		}
		// 存量行按 api_error 归类（幂等：仅覆盖仍为空的行）。
		return LOG_DB.Model(&ErrorEvent{}).Where("event_type = ?", "").Update("event_type", clienterrlog.EventAPIError).Error
	}
	if err := LOG_DB.Exec(`CREATE TABLE IF NOT EXISTS error_events (
		id Int64 DEFAULT 0, created_at Int64, module String, method String, route String,
		status Int32, user_id Int64, username String, token_name String, model_name String,
		channel_id Int64, event_type String DEFAULT 'api_error', task_id String DEFAULT '',
		request_id String, upstream_request_id String, stage String,
		reason String, public_code String, protocol String, elapsed_ms Int64, detail String
	) ENGINE = MergeTree()
	PARTITION BY toYYYYMM(toDateTime(created_at))
	ORDER BY (created_at, request_id)`).Error; err != nil {
		return err
	}
	// 既有表补列：CREATE TABLE IF NOT EXISTS 不会为已存在的表加列。
	if err := LOG_DB.Exec(`ALTER TABLE error_events ADD COLUMN IF NOT EXISTS event_type String DEFAULT 'api_error'`).Error; err != nil {
		return err
	}
	return LOG_DB.Exec(`ALTER TABLE error_events ADD COLUMN IF NOT EXISTS task_id String DEFAULT ''`).Error
}

// GetErrorEvents 按 exact 过滤查询错误事件；排序与分页方式对齐 GetAllLogs，
// ClickHouse 下 id 无业务含义，改用 (created_at, request_id) 排序。
func GetErrorEvents(filter ErrorEventFilter, startIdx, num int) ([]*ErrorEvent, int64, error) {
	if LOG_DB == nil {
		return nil, 0, errors.New("log database unavailable")
	}
	isClickHouse := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	query := LOG_DB.Model(&ErrorEvent{})
	if filter.Module != "" {
		query = query.Where("module = ?", filter.Module)
	}
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if filter.TaskId != "" {
		query = query.Where("task_id = ?", filter.TaskId)
	}
	if filter.Status != nil {
		query = filterErrorEventHTTPStatus(query, *filter.Status)
	}
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	if filter.Username != "" {
		query = query.Where("username = ?", filter.Username)
	}
	if filter.TokenName != "" {
		query = query.Where("token_name = ?", filter.TokenName)
	}
	if filter.ModelName != "" {
		query = query.Where("model_name = ?", filter.ModelName)
	}
	if filter.ChannelId > 0 {
		query = query.Where("channel_id = ?", filter.ChannelId)
	}
	if filter.RequestId != "" {
		query = query.Where("request_id = ?", filter.RequestId)
	}
	if filter.UpstreamRequestId != "" {
		query = query.Where("upstream_request_id = ?", filter.UpstreamRequestId)
	}
	if filter.Reason != "" {
		query = query.Where("reason = ?", filter.Reason)
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", filter.EndTimestamp)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if isClickHouse {
		query = query.Order("created_at DESC").Order("request_id DESC")
	} else {
		query = query.Order("created_at DESC").Order("id DESC")
	}
	events := make([]*ErrorEvent, 0)
	if err := query.Offset(startIdx).Limit(num).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	if err := fillErrorEventChannelNames(events); err != nil {
		return events, total, err
	}
	return events, total, nil
}

// fillErrorEventChannelNames 在应用层回填渠道名；日志库与主库禁止跨库 JOIN
// （与 GetAllLogs 的渠道名回填方式一致）。
func fillErrorEventChannelNames(events []*ErrorEvent) error {
	channelIds := make(map[int]bool, len(events))
	for _, event := range events {
		if event.ChannelId > 0 {
			channelIds[event.ChannelId] = true
		}
	}
	if len(channelIds) == 0 {
		return nil
	}
	ids := make([]int, 0, len(channelIds))
	for id := range channelIds {
		ids = append(ids, id)
	}
	channelNames := make(map[int]string, len(ids))
	if common.MemoryCacheEnabled {
		for _, id := range ids {
			if channel, err := CacheGetChannel(id); err == nil {
				channelNames[id] = channel.Name
			}
		}
	} else {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			channelNames[channel.Id] = channel.Name
		}
	}
	for _, event := range events {
		event.ChannelName = channelNames[event.ChannelId]
	}
	return nil
}
