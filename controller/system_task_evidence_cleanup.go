package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// evidenceRetentionHandler 周期清理过期证据正文：删除对象、保留索引并
// 标记“正文已过期”，不显示成从未收到请求。RetentionDays=0 表示不自动
// 清理（显式的部署决策）。清理不影响任务、资金或对账事实。
type evidenceRetentionHandler struct{}

func (evidenceRetentionHandler) Type() string {
	return model.SystemTaskTypeTaskRequestEvidenceRetention
}

func (evidenceRetentionHandler) Enabled() bool {
	config := system_setting.GetTaskRequestEvidenceConfig()
	return config.Enabled && config.RetentionDays > 0
}

func (evidenceRetentionHandler) Interval() time.Duration {
	return 6 * time.Hour
}

func (evidenceRetentionHandler) NewPayload() any { return nil }

func (evidenceRetentionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	_ = task
	_ = runnerID
	now := common.GetTimestamp()
	for {
		if ctx.Err() != nil {
			return
		}
		expired, err := model.TaskRequestEvidenceExpiredBodies(now, 100)
		if err != nil {
			common.SysError("evidence retention scan failed: " + err.Error())
			return
		}
		if len(expired) == 0 {
			return
		}
		store := service.GetTaskRequestEvidenceStore()
		for _, evidence := range expired {
			events, err := model.ListTaskRequestEvidenceEvents(evidence.Id)
			if err != nil {
				common.SysError("evidence retention list events failed: " + err.Error())
				continue
			}
			deleted := store != nil
			if store != nil {
				for _, event := range events {
					if event.ObjectKey == "" {
						continue
					}
					if err := store.Delete(event.ObjectKey); err != nil {
						deleted = false
						common.SysError("evidence retention delete failed")
					}
				}
			}
			if !deleted {
				return
			}
			if err := model.MarkTaskRequestEvidenceBodyExpired(evidence.Id); err != nil {
				common.SysError("evidence retention mark failed: " + err.Error())
			}
		}
		if len(expired) < 100 {
			return
		}
	}
}
