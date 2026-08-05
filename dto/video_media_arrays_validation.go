package dto

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	VideoMediaArrayImage = "image"
	VideoMediaArrayAudio = "audio"
	VideoMediaArrayVideo = "video"

	maxVideoMediaArrayURLLength = 20 * 1024 * 1024
)

var videoMediaArrayImageMIMETypes = map[string]struct{}{
	"image/jpeg": {}, "image/png": {}, "image/webp": {},
}

// ValidateVideoMediaArrayURL validates the media representation accepted by
// array-based JSON video providers. Durable asset references are allowed only
// before the Link resolver rewrites them to their protected source URL.
func ValidateVideoMediaArrayURL(value, mediaType string, allowAsset bool) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxVideoMediaArrayURLLength {
		return fmt.Errorf("media URL is empty or too large")
	}
	if mediaType != VideoMediaArrayImage && mediaType != VideoMediaArrayAudio && mediaType != VideoMediaArrayVideo {
		return fmt.Errorf("unsupported media type")
	}
	if strings.HasPrefix(value, "asset://") {
		if allowAsset {
			return nil
		}
		return fmt.Errorf("asset URL was not resolved before provider dispatch")
	}
	if strings.HasPrefix(value, "data:") {
		if mediaType != VideoMediaArrayImage {
			return fmt.Errorf("data URL is not supported for this media type")
		}
		comma := strings.IndexByte(value, ',')
		if comma <= len("data:") {
			return fmt.Errorf("invalid data URL")
		}
		metadata := strings.Split(value[len("data:"):comma], ";")
		mime := strings.ToLower(strings.TrimSpace(metadata[0]))
		if _, ok := videoMediaArrayImageMIMETypes[mime]; !ok || len(metadata) != 2 || strings.ToLower(metadata[1]) != "base64" {
			return fmt.Errorf("data URL MIME or encoding is not supported")
		}
		if _, err := base64.StdEncoding.DecodeString(value[comma+1:]); err != nil {
			return fmt.Errorf("data URL payload is not valid base64")
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		if mediaType == VideoMediaArrayImage {
			return fmt.Errorf("media URL must be an http(s) URL or an image data URL")
		}
		return fmt.Errorf("media URL must be an http(s) URL")
	}
	if mediaType != VideoMediaArrayImage && parsed.Scheme != "https" {
		return fmt.Errorf("media URL must be an https URL")
	}
	return nil
}
