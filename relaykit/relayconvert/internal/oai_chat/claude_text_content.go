package oaichat

import (
	"fmt"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// claudeMessageContent preserves explicit text breakpoints for Anthropic
// without changing what other providers receive from Message.ParseContent.
func claudeMessageContent(message *dto.Message) ([]dto.MediaContent, error) {
	blocks, ok := message.Content.([]any)
	if !ok {
		return message.ParseContent(), nil
	}
	for _, raw := range blocks {
		if part, ok := raw.(map[string]any); ok && part["type"] == dto.ContentTypeText {
			if _, hasBreakpoint := part["cache_control"].(map[string]any); hasBreakpoint {
				return claudeContentWithBreakpoints(blocks)
			}
		}
	}
	return message.ParseContent(), nil
}

func claudeContentWithBreakpoints(blocks []any) ([]dto.MediaContent, error) {
	parts := make([]dto.MediaContent, 0, len(blocks))
	for _, raw := range blocks {
		message := dto.Message{Content: []any{raw}}
		for _, part := range message.ParseContent() {
			if fields, ok := raw.(map[string]any); ok && part.Type == dto.ContentTypeText {
				if breakpoint, ok := fields["cache_control"].(map[string]any); ok {
					cacheControl, err := kitutil.Marshal(breakpoint)
					if err != nil {
						return nil, fmt.Errorf("encode Claude text cache_control: %w", err)
					}
					part.CacheControl = cacheControl
				}
			}
			parts = append(parts, part)
		}
	}
	return parts, nil
}
