package clienterrlog

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPExchangeDisplayPreservesNumbersAndUnavailableStates(t *testing.T) {
	for _, body := range []string{
		`{"id":9007199254740993,"n":0,"ratio":1.00e+03,"stream":false}`,
		"{\n  \"id\": 9007199254740993,\n  \"n\": 0,\n  \"ratio\": 1.00e+03,\n  \"stream\": false\n}",
	} {
		exchange := HTTPExchange{
			Request:          HTTPBody{State: "captured", Body: body},
			UpstreamResponse: HTTPBody{State: "too_large", Status: 502},
			Response:         HTTPBody{State: "empty", Status: 400},
		}
		encoded, err := common.Marshal(exchange)
		require.NoError(t, err)
		original := map[string]string{HTTPExchangeDetailKey: string(encoded), "upstream_status": "502", "other": "keep"}
		detail, err := common.Marshal(original)
		require.NoError(t, err)
		var displayed map[string]string
		require.NoError(t, common.UnmarshalJsonStr(FormatHTTPExchangeDetail(string(detail)), &displayed))
		assert.Equal(t, "502", displayed["upstream_status"])
		assert.Equal(t, "keep", displayed["other"])
		var got HTTPExchange
		require.NoError(t, common.UnmarshalJsonStr(displayed[HTTPExchangeDetailKey], &got))
		for _, fragment := range []string{`"id": 9007199254740993`, `"n": 0`, `"ratio": 1.00e+03`, `"stream": false`} {
			assert.Contains(t, got.Request.Body, fragment)
		}
		assert.Contains(t, got.Request.Body, "\n")
		assert.Equal(t, exchange.UpstreamResponse, got.UpstreamResponse)
		assert.Equal(t, exchange.Response, got.Response)
	}
}

func TestHTTPExchangeDisplayMalformedAndExpandedBodies(t *testing.T) {
	for _, tc := range []struct {
		name, body, state string
	}{
		{"malformed body", `{"invalid":`, "unsupported"},
		{"expanded body", "[" + strings.Repeat("[0],", 3000) + "0]", "captured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exchange, err := common.Marshal(HTTPExchange{Request: HTTPBody{State: "captured", Body: tc.body, Status: 400}})
			require.NoError(t, err)
			detail, err := common.Marshal(map[string]string{HTTPExchangeDetailKey: string(exchange), "reason": "keep"})
			require.NoError(t, err)
			var fields map[string]string
			require.NoError(t, common.UnmarshalJsonStr(FormatHTTPExchangeDetail(string(detail)), &fields))
			var got HTTPExchange
			require.NoError(t, common.UnmarshalJsonStr(fields[HTTPExchangeDetailKey], &got))
			assert.Equal(t, tc.state, got.Request.State)
			assert.Equal(t, 400, got.Request.Status)
			assert.Equal(t, "keep", fields["reason"])
			if tc.state == "unsupported" {
				assert.Empty(t, got.Request.Body)
			} else {
				assert.Equal(t, tc.body, got.Request.Body)
			}
		})
	}
	for _, detail := range []string{"", "broken", `{"reason":"keep"}`, `{"http_exchange":"broken","reason":"keep"}`} {
		assert.Equal(t, detail, FormatHTTPExchangeDetail(detail))
	}
}
