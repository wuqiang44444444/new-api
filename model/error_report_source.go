package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// 每小时报告的只读数据源（docs/80-dev/2026-09-17-每小时系统运行与错误邮件报告开发方案.md
// 第 3、4 节）。复用错误事件既有字段合同，不新建采集流程。

// WalkErrorEventsForReport reads one ordered SQL cursor, preserving duplicate
// ClickHouse keys and the half-open window. No offset/keyset re-query changes
// the source set. Channel names are resolved only after closing the cursor,
// including when the main and log databases share a single connection.
func WalkErrorEventsForReport(ctx context.Context, start, end int64, visit func(*ErrorEvent) error) (map[int]string, error) {
	if LOG_DB == nil {
		return nil, errors.New("log database unavailable")
	}
	if start >= end {
		return nil, errors.New("invalid report window")
	}
	query := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).Where("created_at >= ? AND created_at < ?", start, end).Order("created_at ASC")
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		query = query.Order("request_id ASC")
	} else {
		query = query.Order("id ASC")
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var event ErrorEvent
		if err := LOG_DB.ScanRows(rows, &event); err != nil {
			return nil, err
		}
		if event.ChannelId > 0 {
			ids[event.ChannelId] = true
		}
		if len(ids) > ErrorReportMaxSummaryGroups {
			return nil, errors.New("error report channel summary exceeds resource budget")
		}
		if err := visit(&event); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	names := map[int]string{}
	batch := make([]int, 0, 500)
	for id := range ids {
		batch = append(batch, id)
	}
	for start := 0; start < len(batch); start += 500 {
		end := start + 500
		if end > len(batch) {
			end = len(batch)
		}
		var channels []struct {
			Id   int
			Name string
		}
		if err := DB.WithContext(ctx).Table("channels").Select("id, name").Where("id IN ?", batch[start:end]).Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			names[channel.Id] = channel.Name
		}
	}
	return names, nil
}

// Explicit engineering budget: fail the window without advancing or publishing
// partial data if distinct summary dimensions exceed it.
const ErrorReportMaxSummaryGroups = 10000

// GetErrorReportPerfSummaries 按 [start, end) 半开窗口汇总已落库性能统计
// （主库 perf_metrics，bucket_ts 对齐桶起点）。既有 GetPerfMetricsSummaryAll
// 使用闭区间，原样套用到相邻自然小时会把起点恰为 end 的桶重复计入两份
// 报告；小时报告必须使用本函数的半开口径。
func GetErrorReportPerfSummaries(ctx context.Context, start, end int64) ([]PerfMetricSummary, error) {
	var summaries []PerfMetricSummary
	err := DB.WithContext(ctx).Model(&PerfMetric{}).
		Select("model_name, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
		Where("bucket_ts >= ? AND bucket_ts < ?", start, end).
		Group("model_name").
		Having("SUM(request_count) > 0").
		Limit(ErrorReportMaxSummaryGroups + 1).
		Find(&summaries).Error
	if len(summaries) > ErrorReportMaxSummaryGroups {
		return nil, errors.New("error report performance summary exceeds resource budget")
	}
	return summaries, err
}
