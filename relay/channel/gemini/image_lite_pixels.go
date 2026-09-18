package gemini

import (
	"bytes"
	"errors"
	"image"

	"github.com/gin-gonic/gin"
)

// Native delivery verifies an explicit pixel promise without re-encoding the
// provider image. auto does not impose a gateway aspect ratio or pixel matrix.
func validateNativeGeminiImagePixels(c *gin.Context, result GeminiImageResult) error {
	if c == nil {
		return nil
	}
	width, height, err := parsePixelSize(c.GetString(geminiImageSizeKey))
	if err != nil {
		return nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(result.Data))
	if err != nil {
		return errors.New("provider returned invalid image bytes")
	}
	if config.Width != width || config.Height != height {
		return errors.New("provider image does not match the requested native 1024x1024 size")
	}
	return nil
}
