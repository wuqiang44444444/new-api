package relay

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type nativeImageFormatReader struct {
	*io.SectionReader
	path string
}

func (r nativeImageFormatReader) NewReader() (io.ReadCloser, error) { return os.Open(r.path) }

type nativeImageFormatFile struct{ *os.File }

func (f nativeImageFormatFile) Close() error {
	err := f.File.Close()
	removeErr := os.Remove(f.Name())
	if err != nil {
		return err
	}
	return removeErr
}

// Strip only the unsupported provider parameter after native conversion and
// overrides. File ranges preserve media without materializing base64 strings.
// The caller owns the result through its separate closer, including retries.
func prepareNativeImageFormatBody(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (io.Reader, io.Closer, error) {
	request, ok := info.Request.(*dto.ImageRequest)
	if !ok || strings.TrimSpace(request.ResponseFormat) == "" || !nativeImageBase64Only(info) {
		return body, nil, nil
	}
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		return nil, nil, errors.New("invalid image content type")
	}
	output, err := os.CreateTemp("", "image-native-format-*")
	if err != nil {
		return nil, nil, err
	}
	closer := nativeImageFormatFile{output}
	ready := false
	defer func() {
		if !ready {
			_ = closer.Close()
		}
	}()
	writer := imageDeliverySpoolWriter{output}
	if mediaType == "multipart/form-data" {
		multipartWriter := multipart.NewWriter(writer)
		if err := multipartWriter.SetBoundary(params["boundary"]); err != nil {
			return nil, nil, err
		}
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, err
			}
			if part.FormName() == "response_format" && part.FileName() == "" {
				_ = part.Close()
				continue
			}
			destination, err := multipartWriter.CreatePart(part.Header)
			if err != nil {
				return nil, nil, err
			}
			if _, err := io.Copy(destination, part); err != nil {
				return nil, nil, err
			}
		}
		if err := multipartWriter.Close(); err != nil {
			return nil, nil, err
		}
	} else {
		source, err := os.CreateTemp("", "image-native-input-*")
		if err != nil {
			return nil, nil, err
		}
		defer func() { source.Close(); os.Remove(source.Name()) }()
		size, err := io.Copy(imageDeliverySpoolWriter{source}, body)
		if err != nil {
			return nil, nil, err
		}
		fields, err := (imageJSONValue{source: source, size: size}).object()
		if err != nil {
			return nil, nil, err
		}
		delete(fields, "response_format")
		if err := writeImageJSONObject(writer, fields); err != nil {
			return nil, nil, err
		}
	}
	size, err := output.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, err
	}
	ready = true
	return nativeImageFormatReader{SectionReader: io.NewSectionReader(output, 0, size), path: output.Name()}, closer, nil
}
