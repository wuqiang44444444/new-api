package model

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorEventPersistsSanitizedHTTPExchange(t *testing.T) {
	setupErrorEventTestDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	clienterrlog.InstallHTTPExchange(c)
	clienterrlog.ObserveHTTPBody(c.Request.Context(), "request", []byte(`{"model":"test","api_key":"fixture-secret","prompt":"draw a cat"}`), 0)
	event := errorEventFixture(time.Now())
	event.HTTPExchange = clienterrlog.SnapshotHTTPExchange(c.Request.Context())
	require.NoError(t, persistErrorEvent(event))
	events, total, err := GetErrorEvents(ErrorEventFilter{RequestId: event.RequestID}, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, events, 1)
	assert.NotContains(t, events[0].Detail, "fixture-secret")
	var detail map[string]string
	require.NoError(t, common.UnmarshalJsonStr(events[0].Detail, &detail))
	var exchange clienterrlog.HTTPExchange
	require.NoError(t, common.UnmarshalJsonStr(detail[clienterrlog.HTTPExchangeDetailKey], &exchange))
	assert.Equal(t, "captured", exchange.Request.State)
	assert.Contains(t, exchange.Request.Body, "draw a cat")
}

func TestErrorEventHTTPExchangeFitsTextColumnAfterJSONEscaping(t *testing.T) {
	setupErrorEventTestDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	clienterrlog.InstallHTTPExchange(c)
	values := make([]string, 300)
	for i := range values {
		values[i] = strings.Repeat("\\\"", 6)
	}
	body, err := common.Marshal(values)
	require.NoError(t, err)
	for _, kind := range []string{"request", "upstream_request", "upstream_response", "response"} {
		clienterrlog.ObserveHTTPBody(c.Request.Context(), kind, body, 400)
	}
	event := errorEventFixture(time.Now())
	event.HTTPExchange = clienterrlog.SnapshotHTTPExchange(c.Request.Context())
	require.NoError(t, persistErrorEvent(event))
	events, _, err := GetErrorEvents(ErrorEventFilter{RequestId: event.RequestID}, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Less(t, len(events[0].Detail), 65536)
	assert.Equal(t, "too_large", event.HTTPExchange.Request.State)
}
