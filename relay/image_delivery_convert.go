package relay

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// Conversion operates on file ranges. The original spool survives every
// conversion error so the failure response can still deliver generated data.
func (d *imageDelivery) prepare(c *gin.Context) {
	if d == nil || d.err != nil || d.stream {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(system_setting.LoadImageTaskConfig().StoreSeconds)*time.Second)
	defer cancel()
	fields, err := (imageJSONValue{source: d.body, size: d.size}).object()
	if err != nil {
		d.err = err
		return
	}
	results, ok := fields["data"]
	if !ok {
		d.err = errors.New("image results are missing")
		return
	}
	d.results = results
	images, err := results.array()
	if err != nil || len(images) == 0 {
		d.err = errors.New("invalid image results")
		return
	}
	// No output-count policy: a provider's already generated extra image must
	// not be dropped. Request count validation/billing retain their own owner.
	converted, err := os.CreateTemp("", "image-delivery-data-*")
	if err != nil {
		d.err = err
		return
	}
	defer func() { converted.Close(); os.Remove(converted.Name()) }()
	_, d.err = io.WriteString(imageDeliverySpoolWriter{converted}, "[")
	for i, image := range images {
		if d.err != nil {
			return
		}
		var item map[string]imageJSONValue
		item, d.err = image.object()
		if d.err != nil {
			return
		}
		if i > 0 {
			if _, d.err = io.WriteString(imageDeliverySpoolWriter{converted}, ","); d.err != nil {
				return
			}
		}
		var media *os.File
		item, media, d.err = convertImageDeliveryItem(ctx, item, d.format)
		if d.err == nil {
			d.err = writeImageJSONObject(imageDeliverySpoolWriter{converted}, item)
		}
		if media != nil {
			media.Close()
			os.Remove(media.Name())
		}
	}
	if d.err != nil {
		return
	}
	if _, d.err = io.WriteString(imageDeliverySpoolWriter{converted}, "]"); d.err != nil {
		return
	}
	size, err := converted.Seek(0, io.SeekCurrent)
	if err != nil {
		d.err = err
		return
	}
	fields["data"] = imageJSONValue{source: converted, size: size}
	d.output, d.err = os.CreateTemp("", "image-delivery-output-*")
	if d.err == nil {
		d.err = writeImageJSONObject(imageDeliverySpoolWriter{d.output}, fields)
	}
}

// A successful conversion keeps the client's requested shape. On error the
// caller returns the untouched source, explicitly marked as delivery failure.
func convertImageDeliveryItem(ctx context.Context, fields map[string]imageJSONValue, format string) (map[string]imageJSONValue, *os.File, error) {
	encoded, hasEncoded := fields["b64_json"]
	source, hasURL := fields["url"]
	var sourceURL string
	var err error
	if hasURL {
		sourceURL, err = source.text()
		if err != nil {
			return fields, nil, err
		}
		hasURL = sourceURL != ""
	}
	if hasEncoded {
		hasEncoded, err = encoded.hasImageString()
		if err != nil {
			return fields, nil, err
		}
	}
	if format == "url" && hasURL {
		delete(fields, "b64_json")
		return fields, nil, nil
	}
	if format == "b64_json" && hasEncoded {
		delete(fields, "url")
		delete(fields, "url_expires_at")
		return fields, nil, nil
	}
	media, err := os.CreateTemp("", "image-delivery-media-*")
	if err != nil {
		return fields, nil, err
	}
	if hasEncoded {
		var reader io.Reader
		reader, err = encoded.base64Reader()
		if err == nil {
			_, err = io.Copy(imageDeliverySpoolWriter{media}, base64.NewDecoder(base64.StdEncoding, reader))
		}
	} else if hasURL {
		err = retryImageDelivery(ctx, func() error { return downloadImageResultFile(ctx, sourceURL, media) })
	} else {
		err = errors.New("image result is missing")
	}
	if err != nil {
		return fields, media, err
	}
	stat, err := media.Stat()
	if err != nil {
		return fields, media, err
	}
	var prefix [512]byte
	n, err := media.ReadAt(prefix[:], 0)
	if err != nil && err != io.EOF {
		return fields, media, err
	}
	mimeType := http.DetectContentType(prefix[:n])
	if !strings.HasPrefix(mimeType, "image/") {
		return fields, media, errors.New("result is not an image")
	}
	if format == "b64_json" {
		fields["b64_json"] = imageJSONValue{source: media, size: stat.Size(), base64: true}
		delete(fields, "url")
		delete(fields, "url_expires_at")
		return fields, media, nil
	}
	ctx, err = service.WithImageObjectStore(ctx)
	if err != nil {
		return fields, media, err
	}
	key, err := service.BuildImageObjectKey("ephemeral", "result")
	if err != nil {
		return fields, media, err
	}
	err = retryImageDelivery(ctx, func() error { return service.PutImageObjectFile(ctx, key, mimeType, media) })
	if err != nil {
		return fields, media, err
	}
	var url string
	var expires int64
	err = retryImageDelivery(ctx, func() error {
		var signErr error
		url, expires, signErr = service.PresignImageObjectURL(ctx, key)
		return signErr
	})
	if err != nil {
		return fields, media, err
	}
	fields["url"], err = imageJSONLiteral(url)
	if err != nil {
		return fields, media, err
	}
	fields["url_expires_at"], err = imageJSONLiteral(expires)
	delete(fields, "b64_json")
	return fields, media, err
}

// Only result GET and same-key PUT/signing are retried, never generation or
// billing. All attempts share the original delivery deadline and cancellation.
func retryImageDelivery(ctx context.Context, operation func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = operation()
		if err == nil {
			return nil
		}
		var download *imageResultError
		if errors.As(err, &download) && download.DownloadHTTPStatus >= 400 && download.DownloadHTTPStatus < 500 {
			return err
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func downloadImageResultFile(ctx context.Context, sourceURL string, file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return &imageResultError{Code: "result_download_request_failed"}
	}
	client := *service.GetSSRFProtectedHTTPClient()
	client.Timeout = 60 * time.Second
	response, err := client.Do(req)
	service.ObserveImageHTTPExchange(ctx, req, response, err, "result_download")
	if err != nil {
		return &imageResultError{Code: "result_download_transport_failed"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &imageResultError{Code: "result_download_http_error", DownloadHTTPStatus: response.StatusCode}
	}
	if _, err := io.Copy(imageDeliverySpoolWriter{file}, response.Body); err != nil {
		return &imageResultError{Code: "result_download_read_failed", DownloadHTTPStatus: response.StatusCode}
	}
	return nil
}
