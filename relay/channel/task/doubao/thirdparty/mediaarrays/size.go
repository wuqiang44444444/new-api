package mediaarrays

type VideoSize struct {
	Value      string
	Multiplier float64
}

var videoSizes = map[string]VideoSize{
	"720p:16:9": {Value: "1280x720", Multiplier: 1},
	"720p:9:16": {Value: "720x1280", Multiplier: 1},
}

func ResolveVideoSize(resolution, ratio string) (VideoSize, bool) {
	size, ok := videoSizes[resolution+":"+ratio]
	return size, ok
}
