package service

import (
	"errors"
	"mime"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// evidenceContentTypeRequiresJSON 只在 Content-Type 明确声明 JSON 合同时成立；
// 通用脱敏器的 `{`/`[` 嗅探不能证明错误归属客户。
func evidenceContentTypeRequiresJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.Contains(mediaType, "json")
}

// Reserve the durable event before writing its immutable object. A crash leaves an
// unavailable event, never a complete record referring to an uncommitted body.
// 主因/次因同时保留：若脱敏或对象写入先发生故障，随后事件完成更新也失败，则
// 最初处理故障作为主因，完成更新故障作为次因单独保留阶段和 cause，公开类别仍
// 由主因决定；若前序处理成功，只有完成更新失败，则数据库故障为主因。
func persistTaskEvidenceBody(
	event *model.TaskRequestEvidenceEvent,
	body []byte,
	source TaskRequestEvidenceBodySource,
) error {
	phase, complete := event.Phase, event.Complete
	event.Phase, event.Complete = model.TaskRequestEvidencePhaseUnavailable, false
	event.CreatedAt = common.GetTimestamp()
	if err := model.CreateTaskRequestEvidenceEvent(event); err != nil {
		return evidenceUnavailableFailure(
			TaskRequestEvidenceCategoryEventDatabase,
			TaskRequestEvidenceStageEventReserve,
			source,
		).WithCause(err)
	}
	event.Seq = event.Id
	payload, redactErr := evidenceRedactBody(body, event.ContentType)
	var failure *TaskRequestEvidenceError
	if redactErr != nil {
		failure = evidenceRedactionFailure(redactErr, source, evidenceContentTypeRequiresJSON(event.ContentType))
	} else if store := GetTaskRequestEvidenceStore(); store == nil {
		failure = evidenceUnavailableFailure(
			TaskRequestEvidenceCategoryObjectStorage,
			TaskRequestEvidenceStageObjectWrite,
			source,
		).WithCause(errors.New("evidence store not initialized"))
	} else if putErr := store.Put(evidenceObjectKey(event.EvidenceId, event.Id), payload); putErr != nil {
		failure = evidenceStoreWriteFailure(putErr, source)
	}
	if failure == nil {
		event.ObjectKey = evidenceObjectKey(event.EvidenceId, event.Id)
		event.StoredBytes = int64(len(payload))
		event.Sha256 = EvidenceSha256Hex(payload)
		event.Redacted = true
		event.Phase, event.Complete = phase, complete
	}
	if updateErr := model.FinishTaskRequestEvidenceEvent(event); updateErr != nil {
		if failure != nil {
			// 双重故障：主因保持最初处理故障，完成更新失败作为次因保留。
			failure.Secondary = updateErr
			return failure
		}
		return evidenceUnavailableFailure(
			TaskRequestEvidenceCategoryEventDatabase,
			TaskRequestEvidenceStageEventCommit,
			source,
		).WithCause(updateErr)
	}
	if failure != nil {
		return failure
	}
	return nil
}
