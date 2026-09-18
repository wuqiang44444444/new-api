package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomerExportPressureTrackerBackoffAndRecovery(t *testing.T) {
	tracker := &customerExportPressureTracker{}

	// 无等待证据：保持健康。
	tracker.step(0, 0)
	assert.False(t, tracker.degraded)

	// 少量快等待：不触发退让。
	tracker.step(2, 0.1)
	assert.False(t, tracker.degraded)

	// 连接等待均值越过阈值：进入退让。
	tracker.step(10, 4)
	assert.True(t, tracker.degraded)

	// 持续高等待：保持退让。
	tracker.step(20, 10)
	assert.True(t, tracker.degraded)

	// 健康窗口不足：仍退让。
	tracker.step(20, 10)
	assert.True(t, tracker.degraded)
	tracker.step(21, 10.0002)
	assert.True(t, tracker.degraded)

	// 连续健康窗口达到阈值：恢复。
	tracker.step(23, 10.0003)
	assert.False(t, tracker.degraded)

	// 计数器回绕（连接池重建）：按无证据处理，不误判。
	tracker.step(5, 0)
	assert.False(t, tracker.degraded)
}

func TestNormalizeCustomerExportLanguage(t *testing.T) {
	cases := map[string]string{
		"en":      "en",
		"zh":      "zh",
		"zhCN":    "zh",
		"zhTW":    "zh-TW",
		"ZH-CN":   "zh",
		"ZH-TW":   "zh-TW",
		"EN-us":   "en",
		"zh-TW":   "zh-TW",
		"zh-TW ":  "zh-TW",
		"zh-cn":   "zh",
		"zh-HK":   "zh-TW",
		"zh_Hant": "zh-TW",
		"fr-FR":   "fr",
		"ja":      "ja",
		"ru_RU":   "ru",
		"vi-VN":   "vi",
		"de-DE":   "en",
		"":        "en",
		"zz":      "en",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, normalizeCustomerExportLanguage(input), input)
	}
}
