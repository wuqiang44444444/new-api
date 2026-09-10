package seedance

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// Capacity-planning benchmarks for the seedance-link JS hooks (方案 §8.3)：
// 冷装载（编译）、热 hook 执行、观察解析、以及共享引擎超订时的排队表现。
// 数字是容量规划证据，不是通过/失败断言；手工运行：
//
//	go test ./relay/channel/task/seedance -bench BenchmarkSeedancePluginCapacity -run '^$' -benchmem
//
// 排队与执行分开计量：单调用方基准给出纯执行耗时与分配；RunParallel 基准
// 在默认并发 8 信号量上超订调用，ns/op 是聚合吞吐（含等待），验证等待有界、
// 槽位释放、无准入超时。

func benchVideoContract(prompt string) *taskdto.ModelArkVideoCreateRequest {
	duration, resolution, ratio := 4, "720p", "21:9"
	return &taskdto.ModelArkVideoCreateRequest{
		Model:      "customer-model",
		Duration:   &duration,
		Resolution: &resolution,
		Ratio:      &ratio,
		Content: []taskdto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer(prompt)},
			{Type: "image_url", Role: common.GetPointer("reference_image"),
				ImageURL: &taskdto.VideoMediaURL{URL: "https://example.com/reference-frames/bench-input.png"}},
		},
	}
}

func benchPinPlugin(b *testing.B) *pluginruntime.LoadedPlugin {
	b.Helper()
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	if err != nil {
		b.Fatal(err)
	}
	return plugin
}

func benchConversionContext(b *testing.B, plugin *pluginruntime.LoadedPlugin, request *taskdto.ModelArkVideoCreateRequest) *gin.Context {
	b.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/api/v3/contents/generations/tasks", nil)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{
		ContractID: taskdto.VideoContractModelArkV3, ModelArk: request,
	})
	return c
}

// BenchmarkSeedancePluginCapacityColdCompile measures the per-process cold
// cost paid by EnsureSeeded and compileRow before any hook runs: JS runtime
// creation, parse, link, meta decode, and hook validation.
func BenchmarkSeedancePluginCapacityColdCompile(b *testing.B) {
	source := plugins.SeedanceSource()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := CompileSeedanceExtensionSource(source); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeedancePluginCapacityBuildCreateHot measures one hot buildCreate
// execution on an already-compiled engine with a representative feicai
// request (text + one reference image, 720p).
func BenchmarkSeedancePluginCapacityBuildCreateHot(b *testing.B) {
	plugin := benchPinPlugin(b)
	request := benchVideoContract("一只纸船在暴雨的河面上逆流而上，镜头缓慢推近")
	adaptor := &TaskAdaptor{
		protocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
		profile:  dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: feicai.ProviderModelSeedance20Mini720P},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := benchConversionContext(b, plugin, request)
		adaptor.pluginCreate = nil
		if _, err := adaptor.ensureSeedanceCreateConversion(c, info); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeedancePluginCapacityParseObservationHot measures one hot
// parseTaskObservation execution on a realistic succeeded observation.
func BenchmarkSeedancePluginCapacityParseObservationHot(b *testing.B) {
	plugin := benchPinPlugin(b)
	body := `{"id":"fca-20260910-bench-0001","status":"completed","video_url":"https://feicai.example.com/videos/fca-20260910-bench-0001.mp4"}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := plugin.Engine.CallPathWithAdmissionTimeout(
			b.Context(), seedanceExtensionPollAdmissionTimeout,
			"seedance", []string{string(dto.VideoUpstreamProtocolFeicaiVideosV1), "parseTaskObservation"},
			map[string]any{"taskId": "fca-20260910-bench-0001", "body": body},
		)
		if err != nil {
			b.Fatal(err)
		}
		if _, err = decodeSeedanceTaskObservation(result, "fca-20260910-bench-0001", "https://feicai.example.com"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeedancePluginCapacityQueueOversubscribed measures per-call wall
// time when callers oversubscribe the engine's default concurrency-8 slot
// semaphore, proving bounded waiting and slot release under contention.
func BenchmarkSeedancePluginCapacityQueueOversubscribed(b *testing.B) {
	plugin := benchPinPlugin(b)
	body := `{"id":"fca-20260910-bench-0001","status":"completed","video_url":"https://feicai.example.com/videos/fca-20260910-bench-0001.mp4"}`
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		input := map[string]any{"taskId": "fca-20260910-bench-0001", "body": body}
		for pb.Next() {
			result, err := plugin.Engine.CallPathWithAdmissionTimeout(
				b.Context(), seedanceExtensionPollAdmissionTimeout,
				"seedance", []string{string(dto.VideoUpstreamProtocolFeicaiVideosV1), "parseTaskObservation"},
				input,
			)
			if err != nil {
				b.Error(fmt.Errorf("queue bench call failed: %w", err))
				return
			}
			if _, err = decodeSeedanceTaskObservation(result, "fca-20260910-bench-0001", "https://feicai.example.com"); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
