package service

import "net/http"

// The cached general relay client is shared. Infrastructure fetches use a
// shallow copy to clear observation without changing its network policy.
func withoutRequestEvidenceClient(client *http.Client) *http.Client {
	copy := *client
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	copy.Transport = &evidenceInfrastructureTransport{RoundTripper: transport}
	return &copy
}

type evidenceInfrastructureTransport struct{ http.RoundTripper }

func (t *evidenceInfrastructureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.RoundTripper.RoundTrip(req.WithContext(WithoutRequestEvidence(req.Context())))
}
