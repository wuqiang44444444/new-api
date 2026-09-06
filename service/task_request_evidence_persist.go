package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Reserve the durable event before writing its immutable object. A crash leaves an
// unavailable event, never a complete record referring to an uncommitted body.
func persistTaskEvidenceBody(event *model.TaskRequestEvidenceEvent, body []byte) error {
	phase, complete := event.Phase, event.Complete
	event.Phase, event.Complete = model.TaskRequestEvidencePhaseUnavailable, false
	event.CreatedAt = common.GetTimestamp()
	if err := model.CreateTaskRequestEvidenceEvent(event); err != nil {
		return fmt.Errorf("%w: event reservation failed", ErrTaskRequestEvidenceUnavailable)
	}
	event.Seq = event.Id
	payload, err := evidenceRedactBody(body, event.ContentType)
	store := GetTaskRequestEvidenceStore()
	if err == nil && store == nil {
		err = ErrTaskRequestEvidenceUnavailable
	}
	key := evidenceObjectKey(event.EvidenceId, event.Id)
	if err == nil {
		err = store.Put(key, payload)
	}
	if err == nil {
		event.ObjectKey = key
		event.StoredBytes = int64(len(payload))
		event.Sha256 = EvidenceSha256Hex(payload)
		event.Redacted = true
		event.Phase, event.Complete = phase, complete
	}
	if updateErr := model.FinishTaskRequestEvidenceEvent(event); updateErr != nil {
		return fmt.Errorf("%w: event commit failed", ErrTaskRequestEvidenceUnavailable)
	}
	if err != nil {
		return fmt.Errorf("%w: body could not be safely persisted", ErrTaskRequestEvidenceUnavailable)
	}
	return nil
}
