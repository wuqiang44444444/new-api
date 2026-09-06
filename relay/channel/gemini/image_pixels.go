package gemini

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Only equal-ratio scaling is allowed. A provider returning a different ratio
// is a delivery error, never permission to crop, pad or stretch the image.
func normalizeGeminiImagePixels(c *gin.Context, result GeminiImageResult) (GeminiImageResult, error) {
	size := ""
	if c != nil {
		size = c.GetString(geminiImageSizeKey)
	}
	width, height, err := parsePixelSize(size)
	if err != nil {
		return result, nil
	} // auto: no requested pixel dimensions
	config, format, err := image.DecodeConfig(bytes.NewReader(result.Data))
	if err != nil {
		return result, errors.New("provider returned invalid image bytes")
	}
	if config.Width > 8192 || config.Height > 8192 || config.Width*height != config.Height*width {
		return result, errors.New("provider image cannot satisfy the requested size without distortion")
	}
	if config.Width == width && config.Height == height {
		return result, nil
	}
	source, _, err := image.Decode(bytes.NewReader(result.Data))
	if err != nil {
		return result, errors.New("provider image could not be decoded")
	}
	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), draw.Src, nil)
	var output bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&output, target, &jpeg.Options{Quality: 95})
		result.MimeType = "image/jpeg"
	} else {
		err = png.Encode(&output, target)
		result.MimeType = "image/png"
	}
	if err != nil {
		return result, err
	}
	result.Data = output.Bytes()
	return result, nil
}
