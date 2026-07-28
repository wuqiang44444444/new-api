package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
)

func buildChannelTestImageRequest(model string) *dto.ImageRequest {
	size := "1024x1024"
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "nano-banana-2", "gemini-3.1-flash-image-preview-usage":
		size = "1K"
	case "doubao-seedream-4-5-251128":
		size = "2048x2048"
	case "seedream-5", "seedream-5-moxing", "seedream-5-qihang", "seedream-5-0-260128":
		size = "2K"
	}
	return &dto.ImageRequest{
		Model:  model,
		Prompt: "a cute cat",
		N:      lo.ToPtr(uint(1)),
		Size:   size,
	}
}
