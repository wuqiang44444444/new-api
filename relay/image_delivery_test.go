package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFormatPreservesUpstreamEventStream(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{}`))
	info := &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "url"}}
	delivery, deliveryErr := beginImageDelivery(c, info)
	require.Nil(t, deliveryErr)
	require.NotNil(t, delivery)
	defer delivery.cleanup(c)
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Stream-Trace", "kept")
	event := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"fixture\"}\n\n"
	_, err := c.Writer.WriteString(event)
	require.NoError(t, err)
	c.Writer.Flush()
	delivery.prepare(c)
	delivery.send(c)
	assert.Equal(t, event, recorder.Body.String())
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "kept", recorder.Header().Get("X-Stream-Trace"))
	assert.True(t, recorder.Flushed)
}

func TestImageOutputStatFailureReturnsRecoverableDeliveryError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{}`))
	delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "url"}})
	require.Nil(t, deliveryErr)
	defer delivery.cleanup(c)
	_, err := c.Writer.WriteString(`{"data":[{"url":"https://example.test/original.png"}]}`)
	require.NoError(t, err)
	delivery.prepare(c)
	require.NoError(t, delivery.err)
	require.NoError(t, delivery.output.Close())
	delivery.send(c)
	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"image_delivery_failed"`)
	assert.Contains(t, recorder.Body.String(), "https://example.test/original.png")
	assert.Empty(t, recorder.Header().Get("Content-Length"))
}
