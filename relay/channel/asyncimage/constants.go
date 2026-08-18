package asyncimage

const (
	ChannelName = "Async Image Relay"

	nanoBanana2Lite = "nano-banana-2-lite"
	nanoBanana2     = "nano-banana-2"
	seedream5Lite   = "seedream-5.0-lite"
	seedream5Pro    = "seedream-5.0-pro"
)

var ModelList = []string{
	nanoBanana2Lite,
	nanoBanana2,
	seedream5Lite,
	seedream5Pro,
}

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
