package common

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ChatCompletionMessageJSON fills the outbound extension without changing the
// shared Message DTO, which also encodes requests to upstreams.
func ChatCompletionMessageJSON(c *gin.Context, data []byte) []byte {
	if !isChatCompletionDelivery(c) {
		return data
	}
	if patched := completeChatReasoning(string(data), "message"); patched != "" {
		return []byte(patched)
	}
	return data
}

// ChatCompletionDeltaJSON operates on one SSE data payload before framing.
func ChatCompletionDeltaJSON(c *gin.Context, data string) string {
	if isChatCompletionDelivery(c) {
		if patched := completeChatReasoning(data, "delta"); patched != "" {
			return patched
		}
	}
	return data
}

func isChatCompletionDelivery(c *gin.Context) bool {
	return c.Request != nil && c.Request.Method == http.MethodPost &&
		c.FullPath() == "/v1/chat/completions" &&
		c.Writer.Status() >= http.StatusOK && c.Writer.Status() < http.StatusMultipleChoices
}

// A blank result means the original bytes can be written directly. Only the
// missing member is inserted, leaving numbers and unknown fields untouched.
func completeChatReasoning(data, member string) string {
	if !strings.HasPrefix(strings.TrimSpace(data), "{") || !gjson.Valid(data) {
		return ""
	}

	var choices, apiError gjson.Result
	gjson.Parse(data).ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "choices":
			choices = value
		case "error":
			apiError = value
		}
		return true
	})
	if !choices.IsArray() || apiError.Type != gjson.Null {
		return ""
	}

	var output strings.Builder
	cursor := 0
	choices.ForEach(func(_, choice gjson.Result) bool {
		var message gjson.Result
		if !choice.IsObject() {
			return true
		}
		choice.ForEach(func(key, value gjson.Result) bool {
			if key.Str == member {
				message = value
			}
			return true
		})
		if !message.IsObject() {
			return true
		}

		hasMembers := false
		hasReasoningContent := false
		var reasoning gjson.Result
		message.ForEach(func(key, value gjson.Result) bool {
			hasMembers = true
			switch key.Str {
			case "reasoning_content":
				hasReasoningContent = true
				return false
			case "reasoning":
				reasoning = value
			}
			return true
		})
		if hasReasoningContent {
			return true
		}

		value := "null"
		if reasoning.Type == gjson.String && reasoning.Str != "" {
			value = reasoning.Raw
		}
		end := message.Index + len(message.Raw) - 1
		if cursor == 0 {
			output.Grow(len(data) + len(`,"reasoning_content":`) + len(value))
		}
		output.WriteString(data[cursor:end])
		if hasMembers {
			output.WriteByte(',')
		}
		output.WriteString(`"reasoning_content":`)
		output.WriteString(value)
		cursor = end
		return true
	})
	if cursor == 0 {
		return ""
	}
	output.WriteString(data[cursor:])
	return output.String()
}
