package service

import (
	"context"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/model"
)

// One physical exchange per task association. Persist only after the caller has
// consumed/closed the response, retaining the actual request bytes and format.
func DoTaskPollingRequestWithEvidence(client *http.Client, req *http.Request, tasks []*model.Task) (*http.Response, error) {
	var captures []*ImageErrorEvidence
	for _, task := range tasks {
		if task == nil || task.TaskID == "" {
			continue
		}
		index, err := taskMediaEvidenceIndex(task)
		if err != nil || index == nil {
			continue
		}
		capture := newAPIRequestEvidence(*index)
		if capture != nil {
			captures = append(captures, capture)
		}
	}
	if len(captures) == 0 {
		return client.Do(req)
	}
	primary := captures[0]
	req = req.WithContext(context.WithValue(req.Context(), apiRequestEvidenceKey{}, primary))
	observedClient := *client
	observedClient.Transport = WithAPIRequestEvidenceTransport(client.Transport)
	finish := func() {
		for _, capture := range captures {
			if capture != primary {
				capture.records = primary.records
				capture.secrets = primary.secrets
			}
			capture.Finish(false)
		}
	}
	resp, err := observedClient.Do(req)
	if resp == nil || resp.Body == nil {
		finish()
	} else {
		resp.Body = &pollingEvidenceFinishBody{ReadCloser: resp.Body, finish: finish}
	}
	if err != nil {
		finish()
	}
	return resp, err
}

type pollingEvidenceFinishBody struct {
	io.ReadCloser
	finish func()
}

func (b *pollingEvidenceFinishBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish()
	return err
}
