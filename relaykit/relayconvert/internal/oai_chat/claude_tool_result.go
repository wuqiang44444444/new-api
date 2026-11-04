package oaichat

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedclaude "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/claude"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// chatToolResultContentToClaude converts the content inside a tool_result,
// which is separate from the enclosing user message's content blocks.
func chatToolResultContentToClaude(c context.Context, content any) (any, error) {
	if _, ok := content.(string); ok {
		return content, nil
	}
	parts, err := kitutil.Any2Type[[]dto.MediaContent](content)
	if err != nil {
		return nil, fmt.Errorf("invalid tool result content: expected text or a content-part array")
	}
	blocks := make([]dto.ClaudeMediaMessage, 0, len(parts))
	for _, part := range parts {
		if part.Type == dto.ContentTypeText {
			blocks = append(blocks, dto.ClaudeMediaMessage{Type: "text", Text: kitutil.GetPointer(part.Text), CacheControl: part.CacheControl})
			continue
		}
		reason := "Claude tool results cannot carry this content type"
		if part.Type == dto.ContentTypeImageURL {
			// Chat accepts both a URL string and an image_url object.
			if url, ok := part.ImageUrl.(string); ok {
				part.ImageUrl = &dto.MessageImageUrl{Url: url}
			}
			source := part.ToFileSource()
			if source == nil {
				return nil, fmt.Errorf("invalid tool result image: missing image URL")
			}
			data, mime, err := relaymedia.ResolveBase64Data(c, source, "formatting tool result image for Claude")
			if err != nil {
				// Resolver errors may contain signed URLs or media; keep them private.
				return nil, fmt.Errorf("failed to resolve tool result image")
			}
			// Require an identified format; the generic declared-image fallback
			// permits an unknown MIME type that Bedrock cannot consume.
			block, unsupported := sharedclaude.MediaBlock(data, mime, false)
			if unsupported == "" && block.Type == "image" {
				block.CacheControl = part.CacheControl
				blocks = append(blocks, block)
				continue
			}
			reason = "Claude tool result images require JPEG, PNG, GIF, or WebP"
		}
		convdiag.Add(c, types.ConversionDiagnostic{
			Code: "unsupported_media_type", Path: "messages.tool_result.content",
			Message: reason + "; replaced with a text description", Severity: types.ConversionDiagnosticError,
		})
		blocks = append(blocks, dto.ClaudeMediaMessage{Type: "text", Text: kitutil.GetPointer("[Unsupported tool result content]")})
	}
	return blocks, nil
}
