package openai

import (
	"bytes"
	"errors"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// ImageTaskStreamResponse collects final images from an already bounded, fully
// read SSE response. Channel overrides may request SSE even when the customer
// chose task delivery. This does not write a client response or change billing
// ownership. Event names and last-valid usage follow OpenaiImageStreamHandler.
func ImageTaskStreamResponse(info *relaycommon.RelayInfo, body []byte) ([]dto.ImageData, *dto.Usage, error) {
	var images []dto.ImageData
	var usage *dto.Usage
	var event []byte
	for len(body) > 0 || len(event) > 0 {
		var line []byte
		line, body, _ = bytes.Cut(body, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			if len(event) > 0 {
				event = append(event, '\n')
			}
			event = append(event, bytes.TrimPrefix(value, []byte(" "))...)
			continue
		}
		if len(line) != 0 || len(event) == 0 {
			continue
		}
		payload := bytes.TrimSpace(event)
		if bytes.Equal(payload, []byte("[DONE]")) {
			break
		}
		var chunk struct {
			Type string `json:"type"`
			dto.ImageData
		}
		if err := common.Unmarshal(payload, &chunk); err != nil || isOpenAIImageStreamErrorEvent(payload) {
			return nil, nil, errors.New("invalid image stream event")
		}
		if chunk.Type == "image_generation.completed" || chunk.Type == "image_edit.completed" {
			if len(images) >= int(dto.MaxImageN) || (chunk.B64Json == "" && chunk.Url == "") {
				return nil, nil, errors.New("invalid completed image")
			}
			images = append(images, chunk.ImageData)
		}
		parsedUsage, err := ImageTaskUsage(info, payload)
		if err != nil {
			return nil, nil, errors.New("invalid image stream usage")
		}
		if parsedUsage != nil && (usage == nil || service.ValidUsage(parsedUsage)) {
			usage = parsedUsage
		}
		event = event[:0]
	}
	if len(images) == 0 {
		return nil, nil, errors.New("image stream has no completed images")
	}
	return images, usage, nil
}
