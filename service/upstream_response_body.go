package service

import (
	"io"
	"net/http"
)

// OnUpstreamResponseBodyClose keeps the transport cleanup under the evidence
// reader so an unchanged delivery can still reference that same captured body.
func OnUpstreamResponseBodyClose(resp *http.Response, onClose func()) {
	if evidence, ok := resp.Body.(*imageEvidenceBody); ok {
		evidence.ReadCloser = &upstreamBodyOnClose{ReadCloser: evidence.ReadCloser, onClose: onClose}
		return
	}
	resp.Body = &upstreamBodyOnClose{ReadCloser: resp.Body, onClose: onClose}
}

type upstreamBodyOnClose struct {
	io.ReadCloser
	onClose func()
}

func (body *upstreamBodyOnClose) Close() error {
	defer body.onClose()
	return body.ReadCloser.Close()
}
