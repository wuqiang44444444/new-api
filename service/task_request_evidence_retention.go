package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 证据正文保留、读取侧到期判定与写入侧追加保护共用的部分。

// EvidenceRetentionSeconds returns the normal retention policy. Disk pressure
// requests its own fixed-cutoff cleanup and pauses capture independently.
func EvidenceRetentionSeconds() int64 {
	return int64(system_setting.GetTaskRequestEvidenceRetentionPolicy().Days) * 86400
}

// evidenceRetentionDeadline 计算一条证据索引的有效到期时间：取
// "索引创建时间 + 当前配置保留天数"与原有非零更早期限中的较早值；
// 历史 retain_until=0（永久保留）同样受当前上限约束。
func evidenceRetentionDeadline(createdAt, retainUntil, retentionSeconds int64) int64 {
	deadline := createdAt + retentionSeconds
	if retainUntil > 0 && retainUntil < deadline {
		return retainUntil
	}
	return deadline
}

// evidenceBodyExpired 判断证据正文是否已不可读取：到期后或已登记清理申请时
// 立即生效，不等待清理任务；BodyExpired 标记只是清理完成后的持久事实。
func evidenceBodyExpired(index *model.TaskRequestEvidence, now int64) bool {
	if index == nil {
		return true
	}
	if index.BodyExpired || index.CleanupRequestedAt > 0 {
		return true
	}
	deadline := evidenceRetentionDeadline(index.CreatedAt, index.RetainUntil, EvidenceRetentionSeconds())
	return deadline < now
}

// IsEvidenceBodyExpired 是读取侧的到期判定入口：预览与原文/下载接口共用，
// 到期或已申请清理立即返回"正文已过期"，不等物理清理完成。
func IsEvidenceBodyExpired(index *model.TaskRequestEvidence, now int64) bool {
	return evidenceBodyExpired(index, now)
}
