package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Capture only the explicitly selected synchronous delivery. Provider adapters
// still normalize responses and usage; this layer never regenerates an image.
type imageDelivery struct {
	gin.ResponseWriter
	header  http.Header
	body    *os.File
	output  *os.File
	size    int64
	results imageJSONValue
	status  int
	written bool
	err     error
	format  string
	stream  bool
}

func beginImageDelivery(c *gin.Context, info *relaycommon.RelayInfo) (*imageDelivery, *types.NewAPIError) {
	request, ok := info.Request.(*dto.ImageRequest)
	if !ok || strings.TrimSpace(request.ResponseFormat) == "" || (request.Stream != nil && *request.Stream) {
		return nil, nil
	}
	delivery := &imageDelivery{ResponseWriter: c.Writer, header: c.Writer.Header().Clone(), status: http.StatusOK, format: strings.TrimSpace(request.ResponseFormat)}
	delivery.body, delivery.err = os.CreateTemp("", "image-delivery-*")
	if delivery.err != nil {
		return nil, types.NewErrorWithStatusCode(errors.New("image delivery storage is unavailable"), types.ErrorCodeInvalidRequest, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	c.Writer = delivery
	return delivery, nil
}

func (d *imageDelivery) Header() http.Header { return d.header }
func (d *imageDelivery) Status() int         { return d.status }
func (d *imageDelivery) Size() int {
	if !d.written {
		return -1
	}
	return int(d.size)
}
func (d *imageDelivery) Written() bool { return d.written }
func (d *imageDelivery) WriteHeader(status int) {
	if !d.written {
		d.status = status
	}
}
func (d *imageDelivery) WriteHeaderNow() {
	d.written = true
	if strings.HasPrefix(d.header.Get("Content-Type"), "text/event-stream") {
		d.stream = true
		for key, values := range d.header {
			d.ResponseWriter.Header()[key] = values
		}
		d.ResponseWriter.WriteHeader(d.status)
		d.ResponseWriter.WriteHeaderNow()
	}
}
func (d *imageDelivery) Flush() {
	d.WriteHeaderNow()
	if d.stream {
		d.ResponseWriter.Flush()
	}
}
func (d *imageDelivery) WriteString(s string) (int, error) {
	d.WriteHeaderNow()
	if d.stream {
		return d.ResponseWriter.WriteString(s)
	}
	if d.err != nil {
		return len(s), nil
	}
	n, err := io.WriteString(imageDeliverySpoolWriter{d.body}, s)
	d.size += int64(n)
	d.err = err
	return len(s), nil
}
func (d *imageDelivery) Write(p []byte) (int, error) {
	d.WriteHeaderNow()
	if d.stream {
		return d.ResponseWriter.Write(p)
	}
	if d.err != nil {
		return len(p), nil
	}
	n, err := (imageDeliverySpoolWriter{d.body}).Write(p)
	d.size += int64(n)
	d.err = err
	// A local disk error must not make a successful generation retry/refund.
	return len(p), nil
}

func (d *imageDelivery) restore(c *gin.Context) {
	if d != nil {
		c.Writer = d.ResponseWriter
	}
}

// cleanup owns only request-local spools; all exits, including retries and SSE,
// close and delete them. Optional encrypted request evidence is managed by
// the separate evidence store; these response spools are never persisted.
func (d *imageDelivery) cleanup(c *gin.Context) {
	if d == nil {
		return
	}
	d.restore(c)
	for _, file := range []*os.File{d.body, d.output} {
		if file != nil {
			file.Close()
			os.Remove(file.Name())
		}
	}
}

// The generation has already settled. Emit a local delivery error directly so
// controller retry/refund/channel-ban paths cannot regenerate or refund it.
func (d *imageDelivery) send(c *gin.Context) {
	if d == nil || d.stream {
		return
	}
	d.restore(c)
	var stat os.FileInfo
	if d.err == nil {
		if d.output == nil {
			d.err = errors.New("image output is unavailable")
		} else {
			stat, d.err = d.output.Stat()
		}
	}
	if d.err != nil {
		service.RecordImageDeliveryError(c.Request.Context(), "result_delivery", errors.New("image format delivery failed"))
		c.Header("Content-Length", "")
		errorValue, _ := imageJSONLiteral(gin.H{
			"code":    "image_delivery_failed",
			"message": common.MessageWithRequestId("Image generation completed. Format delivery failed; recover the original images from data. Generation was charged; do not regenerate.", c.GetString(common.RequestIdKey)),
		})
		formatValue, _ := imageJSONLiteral(d.format)
		fields := map[string]imageJSONValue{"error": errorValue, "requested_response_format": formatValue}
		if d.results.source != nil {
			fields["data"] = d.results
		}
		c.Header("Content-Type", "application/json")
		c.Status(http.StatusBadGateway)
		_ = writeImageJSONObject(c.Writer, fields)
		return
	}
	for key, values := range d.header {
		c.Writer.Header()[key] = values
	}
	c.Writer.Header().Del("Content-Encoding")
	c.Writer.Header().Del("ETag")
	c.Header("Content-Length", fmt.Sprint(stat.Size()))
	c.Header("Content-Type", "application/json")
	c.Status(d.status)
	_, _ = io.Copy(c.Writer, io.NewSectionReader(d.output, 0, stat.Size()))
}

// Some native response handlers also download URL results. For an explicitly
// selected format, let this delivery own that step so errors cannot regenerate
// or refund an image that already exists. Restore the original billing request.
func (d *imageDelivery) doResponse(c *gin.Context, adaptor channel.Adaptor, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if d != nil && (info.ApiType == constant.APITypeAli || info.ApiType == constant.APITypeReplicate) {
		original := info.Request
		request := *(original.(*dto.ImageRequest))
		request.ResponseFormat = "url"
		info.Request = &request
		defer func() { info.Request = original }()
	}
	return adaptor.DoResponse(c, response, info)
}
