package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 证据正文保留与到期判定工具。仅采集已登记媒体操作；本文件承载
// 七天保留期限计算、读取侧到期判定与写入侧追加保护共用的部分。

// EvidenceRetentionSeconds 返回当前配置的保留秒数；配置越界时按七天兜底，
// 越界配置已在加载期报错，此处只防运行期异常输入。
func EvidenceRetentionSeconds() int64 {
	days := system_setting.GetTaskRequestEvidenceConfig().RetentionDays
	if days < 1 || days > 7 {
		return 7 * 86400
	}
	return int64(days) * 86400
}

// evidenceRetentionDeadline 计算一条证据索引的有效到期时间：取
// "索引创建时间 + 当前配置保留天数"与原有非零更早期限中的较早值；
// 历史 retain_until=0（永久保留）同样受当前上限约束。
func evidenceRetentionDeadline(createdAt, retainUntil int64, retentionDays int) int64 {
	if retentionDays < 1 || retentionDays > 7 {
		retentionDays = 7
	}
	deadline := createdAt + int64(retentionDays)*86400
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
	config := system_setting.GetTaskRequestEvidenceConfig()
	deadline := evidenceRetentionDeadline(index.CreatedAt, index.RetainUntil, config.RetentionDays)
	return deadline < now
}

// IsEvidenceBodyExpired 是读取侧的到期判定入口：预览与原文/下载接口共用，
// 到期或已申请清理立即返回"正文已过期"，不等物理清理完成。
func IsEvidenceBodyExpired(index *model.TaskRequestEvidence, now int64) bool {
	return evidenceBodyExpired(index, now)
}
