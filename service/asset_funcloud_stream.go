package service

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const funCloudMaterialMaxBytes int64 = dto.PublicAssetFunCloudMaxBytes

type funCloudAssetSource struct {
	Body        httpResponseBody
	ContentType string
	Size        int64
	Filename    string
}

type httpResponseBody interface {
	Read([]byte) (int, error)
	Close() error
}

func openFunCloudAssetSource(ctx context.Context, remoteURL, mediaType string) (*funCloudAssetSource, error) {
	protection, err := common.NewSSRFProtectionFromFetchSetting(false, false, false, nil, nil, []string{"443"}, true)
	if err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(remoteURL)
	if err != nil || parsedURL.User != nil || !strings.EqualFold(parsedURL.Scheme, "https") || protection.ValidateURL(remoteURL) != nil {
		return nil, fmt.Errorf("%w: source URL must be safe HTTPS on port 443", ErrInvalidAssetRequest)
	}
	client := newProtectedFetchHTTPClientWithProxy(nil, nil, func() (*common.SSRFProtection, bool, error) {
		return protection, true, nil
	}, func(*http.Request) (*url.URL, error) {
		return nil, nil
	})
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= dto.PublicAssetFunCloudRedirectLimit {
			return fmt.Errorf("asset source redirect limit exceeded")
		}
		if req == nil || req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") || protection.ValidateURL(req.URL.String()) != nil {
			return fmt.Errorf("asset source redirect is unsafe")
		}
		return nil
	}

	return openFunCloudAssetSourceWithClient(ctx, remoteURL, mediaType, client)
}

func openFunCloudAssetSourceWithClient(ctx context.Context, remoteURL, mediaType string, client *http.Client) (*funCloudAssetSource, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: source request is invalid", ErrInvalidAssetRequest)
	}
	request.Header.Set("Accept", acceptedAssetMediaTypes(mediaType))
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: source fetch failed", ErrInvalidAssetRequest)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%w: source fetch returned HTTP %d", ErrInvalidAssetRequest, response.StatusCode)
	}
	if response.ContentLength > funCloudMaterialMaxBytes {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%w: source exceeds configured upload limit", ErrInvalidAssetRequest)
	}

	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !allowedAssetContentType(mediaType, contentType) {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%w: source MIME does not match media_type", ErrInvalidAssetRequest)
	}
	return &funCloudAssetSource{
		Body: response.Body, ContentType: contentType, Size: response.ContentLength,
		Filename: funCloudUploadFilename(contentType),
	}, nil
}

func acceptedAssetMediaTypes(mediaType string) string {
	switch mediaType {
	case "image":
		return "image/jpeg,image/png,image/webp,image/bmp,image/tiff,image/gif"
	case "video":
		return "video/mp4,video/quicktime"
	case "audio":
		return "audio/mpeg,audio/wav,audio/x-wav"
	default:
		return "application/octet-stream"
	}
}

func allowedAssetContentType(mediaType, contentType string) bool {
	allowed := map[string]map[string]struct{}{
		"image": {"image/jpeg": {}, "image/png": {}, "image/webp": {}, "image/bmp": {}, "image/tiff": {}, "image/gif": {}},
		"video": {"video/mp4": {}, "video/quicktime": {}},
		"audio": {"audio/mpeg": {}, "audio/wav": {}, "audio/x-wav": {}},
	}
	_, ok := allowed[mediaType][strings.ToLower(strings.TrimSpace(contentType))]
	return ok
}

func funCloudUploadFilename(contentType string) string {
	extension := map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/bmp": ".bmp",
		"image/tiff": ".tiff", "image/gif": ".gif", "video/mp4": ".mp4", "video/quicktime": ".mov",
		"audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav",
	}[contentType]
	return filepath.Base("upload" + extension)
}
