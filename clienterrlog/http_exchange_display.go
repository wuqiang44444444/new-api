package clienterrlog

import "github.com/QuantumNous/new-api/common"

// FormatHTTPExchangeDetail formats already redacted stored bodies for an admin
// read. It never writes back and preserves numeric lexemes without float casts.
func FormatHTTPExchangeDetail(detail string) string {
	var fields map[string]string
	if common.UnmarshalJsonStr(detail, &fields) != nil || fields[HTTPExchangeDetailKey] == "" {
		return detail
	}
	var exchange HTTPExchange
	if common.UnmarshalJsonStr(fields[HTTPExchangeDetailKey], &exchange) != nil {
		return detail
	}
	for _, body := range []*HTTPBody{&exchange.Request, &exchange.UpstreamRequest, &exchange.UpstreamResponse, &exchange.Response} {
		if body.State != "captured" {
			continue
		}
		formatted, err := common.IndentJson([]byte(body.Body))
		if err != nil {
			body.State, body.Body = "unsupported", ""
			continue
		}
		// Deeply nested compact JSON may expand substantially. Keep its safe
		// compact representation instead of inflating the admin response.
		if len(formatted) <= diagnosticBodyLimit {
			body.Body = string(formatted)
		}
	}
	encoded, err := common.Marshal(exchange)
	if err != nil {
		return detail
	}
	fields[HTTPExchangeDetailKey] = string(encoded)
	encoded, err = common.Marshal(fields)
	if err != nil {
		return detail
	}
	return string(encoded)
}
