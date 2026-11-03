package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func DoTaskPollingRequestWithEvidence(client *http.Client, req *http.Request, tasks []*model.Task) (*http.Response, error) {
	if !evidenceEnabled() || len(tasks) == 0 {
		return client.Do(req)
	}
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: common.GetUUID(), Kind: "task_polling"})
	for _, task := range tasks {
		if task == nil {
			continue
		}
		capture.tasks = append(capture.tasks, model.EvidenceTaskLink{TaskID: task.TaskID, UserID: task.UserId, AppID: task.AppID})
	}
	req = req.WithContext(context.WithValue(req.Context(), apiRequestEvidenceKey{}, capture))
	observed := *client
	observed.Transport = WithAPIRequestEvidenceTransport(client.Transport)
	resp, err := observed.Do(req)
	if resp != nil {
		if body, ok := resp.Body.(*imageEvidenceBody); ok {
			body.finish = func() { capture.Finish(false) }
		} else {
			capture.Finish(false)
		}
	} else {
		capture.Finish(false)
	}
	return resp, err
}
