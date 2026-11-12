package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Bound the extra inspection work on the selection path. A skipped body is
// still parsed and relayed normally by its existing handler.
const maxDefaultAffinityInspectionBytes = 1 << 20

func isSelfContainedAffinityRequest(c *gin.Context, path string) bool {
	if c.ContentType() != gin.MIMEJSON {
		return false
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil || storage.Size() > maxDefaultAffinityInspectionBytes || storage.IsDisk() {
		return false
	}
	body, err := storage.Bytes()
	if err != nil || !gjson.ValidBytes(body) {
		return false
	}
	request := gjson.ParseBytes(body)
	if !request.IsObject() {
		return false
	}
	inputFound, eligible := false, true
	request.ForEach(func(key, value gjson.Result) bool {
		// The relay decoders also accept case-insensitive JSON field names.
		switch strings.ToLower(key.Str) {
		case "previous_response_id", "conversation", "conversation_id", "session_id", "thread_id",
			"container", "container_id", "prompt", "files", "file_id", "file_ids", "vector_store_ids",
			"mcp_servers", "attachments", "documents", "audio", "extra_body":
			eligible = value.Type == gjson.Null || value.Type == gjson.String && strings.TrimSpace(value.Str) == ""
		case "background":
			eligible = value.Type == gjson.False || value.Type == gjson.Null
		case "modalities":
			eligible = value.IsArray() && affinityArrayEvery(value, func(item gjson.Result) bool { return item.Str == "text" })
		case "input":
			if path == "/v1/responses" {
				inputFound = true
				eligible = value.Type == gjson.String || value.IsArray() && isAffinityResponsesInput(value)
			}
		case "messages":
			if path != "/v1/responses" {
				inputFound = true
				eligible = value.IsArray() && affinityArrayEvery(value, isAffinityTextMessage)
			}
		case "system":
			if path == "/v1/messages" {
				eligible = isAffinityTextContent(value, false)
			}
		case "tools":
			eligible = value.Type == gjson.Null || value.IsArray() && affinityArrayEvery(value, func(tool gjson.Result) bool {
				if !tool.IsObject() {
					return false
				}
				switch tool.Get("type").Str {
				case "function", "custom":
					return true
				case "":
					return path == "/v1/messages" && tool.Get("name").Type == gjson.String && tool.Get("input_schema").IsObject()
				default:
					return false
				}
			})
		}
		return eligible
	})
	return inputFound && eligible
}

// Function results are self-contained only when their calls appear earlier
// in the same input. Stored item IDs and non-text results stay out of scope.
func isAffinityResponsesInput(input gjson.Result) bool {
	var calls map[string]bool
	pending := 0
	return affinityArrayEvery(input, func(item gjson.Result) bool {
		kind := item.Get("type").Str
		if kind != "function_call" && kind != "function_call_output" {
			return isAffinityTextMessage(item)
		}
		id := item.Get("call_id")
		if id.Type != gjson.String || strings.TrimSpace(id.Str) == "" || !isAffinityResponsesToolItem(item, kind) {
			return false
		}
		if kind == "function_call" {
			if _, seen := calls[id.Str]; seen || item.Get("name").Type != gjson.String || item.Get("arguments").Type != gjson.String {
				return false
			}
			if calls == nil {
				calls = make(map[string]bool)
			}
			calls[id.Str] = true
			pending++
			return true
		}
		output := item.Get("output")
		if !calls[id.Str] || !output.Exists() || output.Type == gjson.Null || !isAffinityTextContent(output, false) {
			return false
		}
		calls[id.Str] = false
		pending--
		return true
	}) && pending == 0
}

func isAffinityResponsesToolItem(item gjson.Result, kind string) bool {
	seen := make(map[string]bool, 5)
	eligible := true
	item.ForEach(func(key, value gjson.Result) bool {
		if seen[key.Str] {
			eligible = false
			return false
		}
		seen[key.Str] = true
		switch key.Str {
		case "type", "call_id":
		case "status":
			eligible = value.Str == "completed"
		case "name", "arguments":
			eligible = kind == "function_call"
		case "output":
			eligible = kind == "function_call_output"
		default:
			eligible = false
		}
		return eligible
	})
	return eligible
}

func affinityArrayEvery(array gjson.Result, accepts func(gjson.Result) bool) bool {
	eligible := true
	array.ForEach(func(_, item gjson.Result) bool {
		eligible = accepts(item)
		return eligible
	})
	return eligible
}

func isAffinityTextMessage(message gjson.Result) bool {
	if !message.IsObject() || message.Get("id").Exists() {
		return false
	}
	if kind := message.Get("type").Str; kind != "" && kind != "message" {
		return false
	}
	switch message.Get("role").Str {
	case "user", "assistant", "system", "developer", "tool", "function":
		// Inspect every content field, including case variants and duplicates.
		// Only the relay's own decoder decides whether the request is valid.
		eligible := true
		message.ForEach(func(key, value gjson.Result) bool {
			switch strings.ToLower(key.Str) {
			case "content":
				eligible = isAffinityTextContent(value, true)
			case "audio", "file_id", "attachments", "container_id", "id":
				eligible = value.Type == gjson.Null
			}
			return eligible
		})
		return eligible
	default:
		return false
	}
}

func isAffinityTextContent(content gjson.Result, allowToolHistory bool) bool {
	if content.Type == gjson.String || content.Type == gjson.Null {
		return true
	}
	return content.IsArray() && affinityArrayEvery(content, func(part gjson.Result) bool {
		switch part.Get("type").Str {
		case "text", "input_text", "output_text", "refusal":
			return true
		case "tool_use":
			return allowToolHistory
		case "tool_result":
			return allowToolHistory && isAffinityTextContent(part.Get("content"), false)
		default:
			return false
		}
	})
}
