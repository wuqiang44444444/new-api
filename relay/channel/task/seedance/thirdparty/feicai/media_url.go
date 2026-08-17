package feicai

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	mediaImage = "image"
	mediaAudio = "audio"
	mediaVideo = "video"

	maxMediaURLLength = 20 * 1024 * 1024
)

var imageMIMETypes = map[string]struct{}{
	"image/jpeg": {}, "image/png": {}, "image/webp": {},
}

func validateMediaURL(value, mediaType string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxMediaURLLength {
		return fmt.Errorf("media URL is empty or too large")
	}
	if mediaType != mediaImage && mediaType != mediaAudio && mediaType != mediaVideo {
		return fmt.Errorf("unsupported media type")
	}
	if strings.HasPrefix(value, "asset://") {
		if strings.TrimSpace(strings.TrimPrefix(value, "asset://")) == "" {
			return fmt.Errorf("asset URL is invalid")
		}
		return nil
	}
	if strings.HasPrefix(value, "data:") {
		if mediaType != mediaImage {
			return fmt.Errorf("data URL is not supported for this media type")
		}
		comma := strings.IndexByte(value, ',')
		if comma <= len("data:") {
			return fmt.Errorf("invalid data URL")
		}
		metadata := strings.Split(value[len("data:"):comma], ";")
		mime := strings.ToLower(strings.TrimSpace(metadata[0]))
		if _, ok := imageMIMETypes[mime]; !ok || len(metadata) != 2 || strings.ToLower(metadata[1]) != "base64" {
			return fmt.Errorf("data URL MIME or encoding is not supported")
		}
		if _, err := base64.StdEncoding.DecodeString(value[comma+1:]); err != nil {
			return fmt.Errorf("data URL payload is not valid base64")
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		if mediaType == mediaImage {
			return fmt.Errorf("media URL must be an http(s) URL or an image data URL")
		}
		return fmt.Errorf("media URL must be an http(s) URL")
	}
	if mediaType != mediaImage && parsed.Scheme != "https" {
		return fmt.Errorf("media URL must be an https URL")
	}
	return nil
}
