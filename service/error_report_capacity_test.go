package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

// Measure the production spool pipeline with generated source rows and streamed
// output; do not retain input or complete report bodies in the benchmark.
func BenchmarkErrorReportWindowCapacity(b *testing.B) {
	setting := config.GlobalConfig.Get("perf_metrics_setting").(*perf_metrics_setting.PerfMetricsSetting)
	previous := setting.Enabled
	setting.Enabled = false
	b.Cleanup(func() { setting.Enabled = previous })
	for _, size := range []int{1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			detail, err := common.Marshal(map[string]string{"message": strings.Repeat("upstream unavailable ", 20)})
			if err != nil {
				b.Fatal(err)
			}
			h := newErrorReportHandler(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				view, spool, err := h.buildErrorReport(context.Background(), beijingTime(10, 0).Unix(), beijingTime(11, 0).Unix(), func(visit func(*model.ErrorEvent) error) (map[int]string, error) {
					names := map[int]string{}
					for index := 0; index < size; index++ {
						channel := index%20 + 1
						names[channel] = fmt.Sprintf("channel-%d", channel)
						event := &model.ErrorEvent{CreatedAt: beijingTime(10, 0).Unix() + int64(index%3600), EventType: "api_error", Status: 503, Reason: "unclassified", RequestId: fmt.Sprintf("req-%d", index), ChannelId: channel, Detail: string(detail)}
						if err := visit(event); err != nil {
							return nil, err
						}
					}
					return names, nil
				})
				if err != nil {
					b.Fatal(err)
				}
				for partNo := 1; partNo <= view.TotalParts; partNo++ {
					if _, err := spool.nextPart(view, partNo); err != nil {
						spool.file.Close()
						b.Fatal(err)
					}
				}
				spool.file.Close()
				b.ReportMetric(float64(view.TotalParts), "parts/op")
			}
		})
	}
}
