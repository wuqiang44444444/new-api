package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type evidenceBenchmarkWriter struct{ header http.Header }

func (w *evidenceBenchmarkWriter) Header() http.Header         { return w.header }
func (w *evidenceBenchmarkWriter) WriteHeader(int)             {}
func (w *evidenceBenchmarkWriter) Write(p []byte) (int, error) { return len(p), nil }

// Includes every submitted worker write, database transaction and file sync.
// Disabled is the no-evidence cost, not a claim about pre-change performance.
func BenchmarkRequestEvidenceCapture(b *testing.B) {
	for _, size := range []int{4096, 1 << 20} {
		name := "4KiB"
		if size > 4096 {
			name = "1MiB"
		}
		for _, enabled := range []bool{false, true} {
			mode := "disabled"
			if enabled {
				mode = "enabled"
			}
			b.Run(name+"/"+mode, func(b *testing.B) {
				setupEvidenceTestEnv(b)
				config := system_setting.GetTaskRequestEvidenceConfig()
				config.Enabled = enabled
				system_setting.SetTaskRequestEvidenceConfig(config)
				payload := bytes.Repeat([]byte("x"), size)
				engine := gin.New()
				engine.Use(APIRequestEvidence())
				engine.POST("/v1/responses", func(c *gin.Context) {
					_, err := io.Copy(io.Discard, c.Request.Body)
					require.NoError(b, err)
					_, err = c.Writer.Write(payload)
					require.NoError(b, err)
				})
				beforeFailures := imageEvidenceFailed.Load() + imageEvidenceDropped.Load()
				b.ReportAllocs()
				b.SetBytes(int64(2 * size))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					req := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(payload))
					engine.ServeHTTP(&evidenceBenchmarkWriter{header: make(http.Header)}, req)
					if enabled {
						require.NoError(b, FlushRequestEvidence(context.Background()))
					}
				}
				b.StopTimer()
				require.Equal(b, beforeFailures, imageEvidenceFailed.Load()+imageEvidenceDropped.Load(), "benchmark cannot improve results by losing evidence")
			})
		}
	}
	for _, mode := range []string{"segmented", "per_message_reference"} {
		b.Run("websocket_20x1KiB/"+mode, func(b *testing.B) {
			setupEvidenceTestEnv(b)
			payload := bytes.Repeat([]byte("x"), 1024)
			beforeFailures := imageEvidenceFailed.Load() + imageEvidenceDropped.Load()
			b.ReportAllocs()
			b.SetBytes(20 * 1024)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "realtime"})
				ctx := WithImageErrorEvidence(context.Background(), capture)
				for j := 0; j < 20; j++ {
					if mode == "per_message_reference" {
						// Prior granularity, benchmark only; no production fallback.
						record := capture.newRecord("realtime_north_receive", EvidenceObjectMeta{MessageType: websocket.TextMessage}, false)
						record.append(payload)
						record.close()
					} else {
						ObserveRealtimeFrame(ctx, "realtime_north_receive", websocket.TextMessage, payload, nil)
					}
				}
				capture.Finish(false)
				require.NoError(b, FlushRequestEvidence(context.Background()))
			}
			b.StopTimer()
			require.Equal(b, beforeFailures, imageEvidenceFailed.Load()+imageEvidenceDropped.Load())
			var count int64
			require.NoError(b, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&count).Error)
			expected := int64(b.N)
			if mode == "per_message_reference" {
				expected *= 20
			}
			require.Equal(b, expected, count)
			b.ReportMetric(float64(count)/float64(b.N), "events/op")
		})
	}
}
