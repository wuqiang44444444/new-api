package system_setting

import "github.com/QuantumNous/new-api/common"

// 显式图片执行（image_openai_v1）的容量与时间预算（G12 待容量测试冻结；
// 以下为文档化默认值，非价格配置）。
const (
	ImageAsyncMaxWaitingEnv      = "IMAGE_ASYNC_MAX_WAITING"
	ImageAsyncMaxPerAppEnv       = "IMAGE_ASYNC_MAX_PER_APP"
	ImageAsyncExecConcurrencyEnv = "IMAGE_ASYNC_EXECUTE_CONCURRENCY"
	ImageAsyncChannelExecEnv     = "IMAGE_ASYNC_CHANNEL_EXECUTE_CONCURRENCY"
	ImageAsyncQueueSecondsEnv    = "IMAGE_ASYNC_QUEUE_SECONDS"
	ImageAsyncExecuteSecondsEnv  = "IMAGE_ASYNC_EXECUTE_SECONDS"
	ImageAsyncStoreSecondsEnv    = "IMAGE_ASYNC_STORE_SECONDS"
	ImageAsyncWorkerBatchEnv     = "IMAGE_ASYNC_WORKER_BATCH"
)

const (
	DefaultImageAsyncMaxWaiting      = 1000
	DefaultImageAsyncMaxPerApp       = 20
	DefaultImageAsyncExecConcurrency = 8
	DefaultImageAsyncChannelExec     = 4
	DefaultImageAsyncQueueSeconds    = 30 * 60
	DefaultImageAsyncExecuteSeconds  = 10 * 60
	DefaultImageAsyncStoreSeconds    = 5 * 60
	DefaultImageAsyncWorkerBatch     = 8
)

type ImageTaskConfig struct {
	MaxWaiting      int
	MaxPerApp       int
	ExecConcurrency int
	ChannelExec     int
	QueueSeconds    int64
	ExecuteSeconds  int64
	StoreSeconds    int64
	WorkerBatch     int
}

// LoadImageTaskConfig reads positive capacity/budget settings; invalid or
// non-positive values fall back to the documented defaults.
func LoadImageTaskConfig() ImageTaskConfig {
	return ImageTaskConfig{
		MaxWaiting:      positiveOrDefault(common.GetEnvOrDefault(ImageAsyncMaxWaitingEnv, DefaultImageAsyncMaxWaiting), DefaultImageAsyncMaxWaiting),
		MaxPerApp:       positiveOrDefault(common.GetEnvOrDefault(ImageAsyncMaxPerAppEnv, DefaultImageAsyncMaxPerApp), DefaultImageAsyncMaxPerApp),
		ExecConcurrency: positiveOrDefault(common.GetEnvOrDefault(ImageAsyncExecConcurrencyEnv, DefaultImageAsyncExecConcurrency), DefaultImageAsyncExecConcurrency),
		ChannelExec:     positiveOrDefault(common.GetEnvOrDefault(ImageAsyncChannelExecEnv, DefaultImageAsyncChannelExec), DefaultImageAsyncChannelExec),
		QueueSeconds:    int64(positiveOrDefault(common.GetEnvOrDefault(ImageAsyncQueueSecondsEnv, int(DefaultImageAsyncQueueSeconds)), int(DefaultImageAsyncQueueSeconds))),
		ExecuteSeconds:  int64(positiveOrDefault(common.GetEnvOrDefault(ImageAsyncExecuteSecondsEnv, int(DefaultImageAsyncExecuteSeconds)), int(DefaultImageAsyncExecuteSeconds))),
		StoreSeconds:    int64(positiveOrDefault(common.GetEnvOrDefault(ImageAsyncStoreSecondsEnv, int(DefaultImageAsyncStoreSeconds)), int(DefaultImageAsyncStoreSeconds))),
		WorkerBatch:     positiveOrDefault(common.GetEnvOrDefault(ImageAsyncWorkerBatchEnv, DefaultImageAsyncWorkerBatch), DefaultImageAsyncWorkerBatch),
	}
}

func positiveOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
