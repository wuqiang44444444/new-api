package seedance

import "strings"

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
	"doubao-seedance-2-5-260628",
	"doubao-seedance-2-0-260128-tokensave",
	"doubao-seedance-2-0-260128-moxing",
	"doubao-seedance-2-0-fast-260128-moxing",
	"doubao-seedance-2-0-mini-260615-moxing",
	"doubao-seedance-2-5-260628-moxing",
	"seedance-2-funcloud",
	"seedance-2-fast-funcloud",
	"seedance-2-mini-funcloud",
	"seedance-2-5-funcloud",
}

var ChannelName = "seedance-link"

type videoPriceKey struct {
	is1080p  bool
	is4k     bool
	hasVideo bool
}

var videoPriceTable = map[string]map[videoPriceKey]float64{
	"doubao-seedance-2-0-260128": {
		{hasVideo: false}:                46.0,
		{hasVideo: true}:                 28.0,
		{is1080p: true, hasVideo: false}: 51.0,
		{is1080p: true, hasVideo: true}:  31.0,
		{is4k: true, hasVideo: false}:    26.0,
		{is4k: true, hasVideo: true}:     16.0,
	},
	"doubao-seedance-2-0-fast-260128": {
		{hasVideo: false}: 37.0,
		{hasVideo: true}:  22.0,
	},
}

func GetVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	prices, ok := videoPriceTable[modelName]
	base := prices[videoPriceKey{}]
	if !ok || base <= 0 {
		return 0, false
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	price, ok := prices[videoPriceKey{is1080p: resolution == "1080p", is4k: resolution == "4k", hasVideo: hasVideo}]
	if !ok {
		return 1.0, true
	}
	return price / base, true
}
