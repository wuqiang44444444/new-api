package relay

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 音频入口的证据类别推导（TTS / 转写 / 翻译），供 AudioHelper 窄调用。
func evidenceKindForAudioPath(c *gin.Context) string {
	path := c.Request.URL.Path
	switch {
	case strings.Contains(path, "/audio/speech"):
		return model.TaskRequestEvidenceKindAudioSpeech
	case strings.Contains(path, "/audio/translations"):
		return model.TaskRequestEvidenceKindAudioTranslate
	default:
		return model.TaskRequestEvidenceKindAudioTranscribe
	}
}
