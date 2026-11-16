package openai

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Use the same adapter and fixtures for both routes. Only the public route enables the extension.
func BenchmarkChatCompletionReasoningHTTP(b *testing.B) {
	const frame = `data: {"id":"fixture","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"short"},"finish_reason":null}]}` + "\n\n"
	const frames = 64
	const done = "data: [DONE]"
	fixture := strings.Repeat(frame, frames) + done + "\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, fixture)
	}))
	b.Cleanup(upstream.Close)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	b.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	engine := gin.New()
	handler := func(c *gin.Context) {
		resp, err := upstream.Client().Get(upstream.URL)
		if err != nil {
			c.Status(http.StatusBadGateway)
			return
		}
		info := &relaycommon.RelayInfo{
			RelayFormat: types.RelayFormatOpenAI,
			IsStream:    true,
			DisablePing: true,
			StartTime:   time.Now(),
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture"},
		}
		_, _ = OaiStreamHandler(c, info, resp)
	}
	engine.POST("/pg/chat/completions", handler)
	engine.POST("/v1/chat/completions", handler)
	gateway := httptest.NewServer(engine)
	b.Cleanup(gateway.Close)

	for _, test := range []struct {
		name      string
		path      string
		wantAdded int
	}{
		{name: "original_write", path: "/pg/chat/completions"},
		{name: "with_extension", path: "/v1/chat/completions", wantAdded: len(`,"reasoning_content":null`) * frames},
	} {
		b.Run(test.name, func(b *testing.B) {
			var first, between []time.Duration
			b.ReportAllocs()
			for b.Loop() {
				start := time.Now()
				resp, err := gateway.Client().Post(gateway.URL+test.path, "application/json", nil)
				require.NoError(b, err)
				scanner := bufio.NewScanner(resp.Body)
				count, added := 0, 0
				var last time.Time
				for scanner.Scan() {
					line := scanner.Bytes()
					if len(line) == 0 || string(line) == done {
						continue
					}
					received := time.Now()
					if count == 0 {
						first = append(first, received.Sub(start))
					} else {
						between = append(between, received.Sub(last))
					}
					last = received
					count++
					added += len(line) - (len(frame) - 2)
				}
				scanErr := scanner.Err()
				require.NoError(b, resp.Body.Close())
				require.NoError(b, scanErr)
				require.Equal(b, http.StatusOK, resp.StatusCode)
				require.Equal(b, frames, count)
				require.Equal(b, test.wantAdded, added)
			}
			slices.Sort(first)
			slices.Sort(between)
			b.ReportMetric(float64(first[(len(first)-1)*95/100].Nanoseconds()), "first_p95_ns")
			b.ReportMetric(float64(between[(len(between)-1)*95/100].Nanoseconds()), "between_p95_ns")
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "requests/s")
			b.ReportMetric(float64(test.wantAdded), "added_B/response")
		})
	}
}
