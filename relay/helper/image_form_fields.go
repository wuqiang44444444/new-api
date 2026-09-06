package helper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Preserve the same optional fields as JSON; the image contract owns validation.
func parseImageFormFields(form url.Values, request *dto.ImageRequest) error {
	request.ResponseFormat = form.Get("response_format")
	for _, field := range []struct {
		name   string
		target *json.RawMessage
	}{
		{"style", &request.Style}, {"user", &request.User}, {"background", &request.Background},
		{"moderation", &request.Moderation}, {"output_format", &request.OutputFormat},
		{"output_compression", &request.OutputCompression}, {"input_fidelity", &request.InputFidelity},
		{"watermark_enabled", &request.WatermarkEnabled}, {"user_id", &request.UserId},
		{"images", &request.Images}, {"partial_images", &request.PartialImages}, {"image", &request.Image},
	} {
		if value := form.Get(field.name); value != "" {
			*field.target = json.RawMessage(strconv.Quote(value))
		}
	}
	if value := strings.TrimSpace(form.Get("stream")); value != "" {
		stream, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid stream value: %w", err)
		}
		request.Stream = common.GetPointer(stream)
	}
	return nil
}
