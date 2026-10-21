package clienterrlog

import "github.com/QuantumNous/new-api/common"

type frozenHTTPExchange struct {
	bodies [4]bodyObservation
}

// MarshalJSON is the only conversion from private frozen bytes to a stored
// diagnostic snapshot. The sink's persister invokes it; even an accidental
// serialization of the pending snapshot cannot expose unredacted bytes.
func (exchange HTTPExchange) MarshalJSON() ([]byte, error) {
	type storedHTTPExchange HTTPExchange
	stored := storedHTTPExchange(exchange)
	if frozen := exchange.pending; frozen != nil {
		stored.Request = sanitizeHTTPBody(&frozen.bodies[0])
		stored.UpstreamRequest = sanitizeHTTPBody(&frozen.bodies[1])
		stored.UpstreamResponse = sanitizeHTTPBody(&frozen.bodies[2])
		stored.Response = sanitizeHTTPBody(&frozen.bodies[3])
		stored.pending = nil
	}
	return common.Marshal(stored)
}
