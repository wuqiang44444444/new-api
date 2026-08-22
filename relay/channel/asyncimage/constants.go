package asyncimage

import "github.com/QuantumNous/new-api/constant"

const (
	ChannelName = "FunCloud Async Image"

	nanoBanana2Lite = constant.FunCloudImageProviderModelNanoBanana2Lite
	nanoBanana2     = constant.FunCloudImageProviderModelNanoBanana2
	seedream5Lite   = constant.FunCloudImageProviderModelSeedream5Lite
	seedream5Pro    = constant.FunCloudImageProviderModelSeedream5Pro
)

var ModelList = constant.FunCloudImageProviderModels()

var allAspectRatios = map[string]struct{}{
	"auto": {}, "1:1": {}, "1:4": {}, "16:9": {}, "1:8": {}, "21:9": {},
	"2:3": {}, "3:2": {}, "3:4": {}, "4:1": {}, "4:3": {}, "4:5": {},
	"5:4": {}, "8:1": {}, "9:16": {},
}

var seedreamLiteAspectRatios = map[string]struct{}{
	"1:1": {}, "4:3": {}, "3:4": {}, "16:9": {}, "9:16": {}, "2:3": {}, "3:2": {}, "21:9": {},
}

var seedreamProAspectRatios = map[string]struct{}{
	"1:1": {}, "4:3": {}, "3:4": {}, "16:9": {}, "9:16": {}, "2:3": {}, "3:2": {},
}
