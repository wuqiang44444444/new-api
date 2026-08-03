package dto

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const (
	JSONVideoMediaImage = "image"
	JSONVideoMediaAudio = "audio"

	maxJSONVideoMediaURLLength = 20 * 1024 * 1024
)

var jsonVideoMediaMIMETypes = map[string]map[string]struct{}{
	JSONVideoMediaImage: {
		"image/jpeg": {}, "image/png": {}, "image/webp": {},
	},
	JSONVideoMediaAudio: {
		"audio/mpeg": {}, "audio/wav": {}, "audio/x-wav": {}, "audio/mp4": {},
		"audio/aac": {}, "audio/ogg": {}, "audio/flac": {},
	},
}

// ValidateJSONVideoMediaURL is the pre-selection and adapter-shared media
// contract for the JSON omni-reference profile.
func ValidateJSONVideoMediaURL(value, mediaType string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxJSONVideoMediaURLLength {
		return fmt.Errorf("media URL is empty or too large")
	}
	allowedMIMEs, ok := jsonVideoMediaMIMETypes[mediaType]
	if !ok {
		return fmt.Errorf("unsupported media type")
	}
	if strings.HasPrefix(value, "asset://") {
		return fmt.Errorf("asset URLs are not supported by this profile")
	}
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma <= len("data:") {
			return fmt.Errorf("invalid data URL")
		}
		metadata := strings.Split(value[len("data:"):comma], ";")
		mime := strings.ToLower(strings.TrimSpace(metadata[0]))
		if _, ok := allowedMIMEs[mime]; !ok || len(metadata) != 2 || strings.ToLower(metadata[1]) != "base64" {
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
		return fmt.Errorf("media URL must be an http(s) or data URL")
	}
	return nil
}
