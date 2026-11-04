package relayconvert

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatToClaudeToolResultImages(t *testing.T) {
	for _, history := range []string{"assistant tool call", "user text", "user blocks", "previous tool result"} {
		for _, url := range []string{"data:image/png;base64,aW1hZ2U=", "https://example.test/screenshot.png"} {
			t.Run(history+"/"+url[:4], func(t *testing.T) {
				calls := 0
				SetMediaResolver(MediaResolver{GetBase64Data: func(_ context.Context, source types.FileSource, _ ...string) (string, string, error) {
					calls++
					assert.Equal(t, url, source.GetRawData())
					assert.Equal(t, url[:4] == "http", source.IsURL())
					return "aW1hZ2U=", "image/png", nil
				}})
				t.Cleanup(func() { SetMediaResolver(MediaResolver{}) })
				messages := []dto.Message{{Role: "user", Content: "take a screenshot"}}
				switch history {
				case "assistant tool call", "previous tool result":
					messages = append(messages, dto.Message{Role: "assistant", ToolCalls: []byte(`[{"id":"call_image","type":"function","function":{"name":"screenshot","arguments":"{}"}}]`)})
					if history == "previous tool result" {
						messages[len(messages)-1].ToolCalls = []byte(`[{"id":"call_text","type":"function","function":{"name":"read_text","arguments":"{}"}},{"id":"call_image","type":"function","function":{"name":"screenshot","arguments":"{}"}}]`)
						messages = append(messages, dto.Message{Role: "tool", ToolCallId: "call_text", Content: "earlier result"})
					}
				case "user blocks":
					messages[0].Content = []any{map[string]any{"type": "text", "text": "take a screenshot"}}
				}
				messages = append(messages, dto.Message{Role: "tool", ToolCallId: "call_image", Content: []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}, "cache_control": map[string]any{"type": "ephemeral"}},
					map[string]any{"type": "text", "text": "screenshot captured", "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
				}})
				max := uint(1024)
				result, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, &dto.GeneralOpenAIRequest{Model: "claude-test", MaxTokens: &max, Messages: messages})
				require.NoError(t, err)
				require.Equal(t, 1, calls)
				request := result.Value.(*dto.ClaudeRequest)
				last := request.Messages[len(request.Messages)-1]
				assert.Equal(t, "user", last.Role)
				blocks, err := last.ParseContent()
				require.NoError(t, err)
				switch history {
				case "assistant tool call":
					require.Len(t, request.Messages, 3)
					require.Len(t, blocks, 1)
				case "previous tool result":
					require.Len(t, request.Messages, 3)
					require.Len(t, blocks, 2)
				default:
					require.Len(t, request.Messages, 1)
					require.Len(t, blocks, 2)
					assert.Equal(t, "take a screenshot", blocks[0].GetText())
				}
				tool := blocks[len(blocks)-1]
				assert.Equal(t, "tool_result", tool.Type)
				assert.Equal(t, "call_image", tool.ToolUseId)
				content, err := kitutil.Any2Type[[]dto.ClaudeMediaMessage](tool.Content)
				require.NoError(t, err)
				require.Len(t, content, 2)
				assert.Equal(t, "image", content[0].Type)
				assert.Equal(t, &dto.ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: "aW1hZ2U="}, content[0].Source)
				assert.Equal(t, "screenshot captured", content[1].GetText())
				assert.JSONEq(t, `{"type":"ephemeral"}`, string(content[0].CacheControl))
				assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(content[1].CacheControl))
				if history == "previous tool result" {
					assert.Equal(t, "earlier result", blocks[0].Content)
				}
				body, err := kitutil.Marshal(request)
				require.NoError(t, err)
				assert.NotContains(t, string(body), `"image_url"`)
				assert.NotContains(t, string(body), "https://example.test/")
			})
		}
	}
}

func TestChatToClaudeToolResultContentBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name       string
		content    any
		mime       string
		resolveErr error
		wantError  bool
		wantText   string
	}{
		{name: "string unchanged", content: "literal image_url text", wantText: "literal image_url text"},
		{name: "unsupported image", content: []dto.MediaContent{{Type: "image_url", ImageUrl: "https://example.test/a.svg"}}, mime: "image/svg+xml", wantText: "[Unsupported tool result content]"},
		{name: "unknown image format", content: []dto.MediaContent{{Type: "image_url", ImageUrl: "https://example.test/a"}}, wantText: "[Unsupported tool result content]"},
		{name: "blank MIME type", content: []dto.MediaContent{{Type: "image_url", ImageUrl: "https://example.test/a"}}, mime: " ; charset=binary", wantText: "[Unsupported tool result content]"},
		{name: "PDF declared as image", content: []dto.MediaContent{{Type: "image_url", ImageUrl: "https://example.test/a.pdf"}}, mime: "application/pdf", wantText: "[Unsupported tool result content]"},
		{name: "unknown block", content: []dto.MediaContent{{Type: "unknown"}}, wantText: "[Unsupported tool result content]"},
		{name: "missing URL", content: []dto.MediaContent{{Type: "image_url"}}, wantError: true},
		{name: "resolver failure", content: []dto.MediaContent{{Type: "image_url", ImageUrl: "https://example.test/a"}}, resolveErr: errors.New("private URL should not escape"), wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			SetMediaResolver(MediaResolver{GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
				return "aW1hZ2U=", tt.mime, tt.resolveErr
			}})
			t.Cleanup(func() { SetMediaResolver(MediaResolver{}) })
			max := uint(1024)
			result, err := ConvertRequest(context.Background(), nil, types.RelayFormatClaude, &dto.GeneralOpenAIRequest{Model: "claude-test", MaxTokens: &max, Messages: []dto.Message{{Role: "user", Content: "check"}, {Role: "tool", ToolCallId: "call_1", Content: tt.content}}})
			if tt.wantError {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "private URL")
				return
			}
			require.NoError(t, err)
			request := result.Value.(*dto.ClaudeRequest)
			blocks, err := request.Messages[len(request.Messages)-1].ParseContent()
			require.NoError(t, err)
			tool := blocks[len(blocks)-1]
			if _, ok := tt.content.(string); ok {
				assert.Equal(t, tt.wantText, tool.Content)
				return
			}
			content, err := kitutil.Any2Type[[]dto.ClaudeMediaMessage](tool.Content)
			require.NoError(t, err)
			require.Len(t, content, 1)
			assert.Equal(t, "text", content[0].Type)
			assert.Equal(t, tt.wantText, content[0].GetText())
			require.Len(t, result.Diagnostics, 1)
			assert.Equal(t, "unsupported_media_type", result.Diagnostics[0].Code)
		})
	}
}
