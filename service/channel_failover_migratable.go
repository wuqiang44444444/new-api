package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const responseOwnerTTL = 24 * time.Hour

const (
	failoverText  = "text"
	failoverTools = "function_tools"
)

type failoverRequestContent struct {
	kind            string
	claudeKeepAll   bool
	ownerReferences []string
}

// Inspect migration separately from default affinity. Unknown request shapes
// can still use their original channel, but cannot authorize a replay.
func inspectFailoverContent(c *gin.Context, operation string) failoverRequestContent {
	var content failoverRequestContent
	// Known continuation headers must be checked even when the body is large.
	if state := c.GetHeader("X-Codex-Turn-State"); state != "" {
		content.ownerReferences = append(content.ownerReferences, "turn:"+state)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return content
	}
	if storage.IsDisk() || storage.Size() > maxDefaultAffinityInspectionBytes {
		// These requests cannot migrate. Decode a narrow projection to detect
		// references without copying a disk-backed body to a byte slice.
		var references struct {
			PreviousResponseID string `json:"previous_response_id"`
			Conversation       any    `json:"conversation"`
			Container          any    `json:"container"`
			Input              []struct {
				Type             string `json:"type"`
				ID               string `json:"id"`
				FileID           string `json:"file_id"`
				EncryptedContent string `json:"encrypted_content"`
				Content          []struct {
					FileID string `json:"file_id"`
				} `json:"content"`
			} `json:"input"`
		}
		if operation == "responses" {
			// A type mismatch elsewhere in the projection does not erase an
			// already decoded continuation field.
			_ = common.UnmarshalBodyReusable(c, &references)
			if references.PreviousResponseID != "" {
				content.ownerReferences = append(content.ownerReferences, "response:"+references.PreviousResponseID)
			}
			if references.Conversation != nil || references.Container != nil {
				content.ownerReferences = append(content.ownerReferences, "")
			}
			for _, item := range references.Input {
				if item.ID != "" || item.FileID != "" || item.EncryptedContent != "" || item.Type == "item_reference" {
					content.ownerReferences = append(content.ownerReferences, "")
				}
				for _, part := range item.Content {
					if part.FileID != "" {
						content.ownerReferences = append(content.ownerReferences, "")
					}
				}
			}
		}
		return content
	}
	body, err := storage.Bytes()
	if err != nil || !gjson.ValidBytes(body) {
		return content
	}

	valid := c.ContentType() == gin.MIMEJSON
	inputFound := false
	kind := failoverText
	contextSeen := false
	request := gjson.ParseBytes(body)
	request.ForEach(func(key, value gjson.Result) bool {
		field := strings.ToLower(key.Str)
		if field != key.Str {
			valid = false
		}
		switch field {
		case "previous_response_id":
			if value.Str != "" {
				content.ownerReferences = append(content.ownerReferences, "response:"+value.Str)
			} else if failoverValuePresent(value) {
				content.ownerReferences = append(content.ownerReferences, "")
			}
			valid = valid && (value.Type == gjson.Null || value.Type == gjson.String)
		case "conversation", "conversation_id", "session_id", "thread_id", "container", "container_id", "file_id", "file_ids",
			"files", "vector_store_ids", "attachments", "encrypted_content", "prompt", "mcp_servers":
			if failoverValuePresent(value) {
				content.ownerReferences = append(content.ownerReferences, "")
			}
		case "input":
			if operation != "responses" {
				valid = false
				break
			}
			inputFound = true
			if value.IsArray() && isAffinityResponsesInput(value) {
				value.ForEach(func(_, item gjson.Result) bool {
					switch item.Get("type").Str {
					case "function_call", "function_call_output":
						kind = failoverTools
					default:
						valid = inspectFailoverMessage(item, operation, &content, &kind) && valid
					}
					return true
				})
			} else if value.IsArray() {
				value.ForEach(func(_, item gjson.Result) bool {
					inspectFailoverReferences(item, &content)
					return true
				})
				valid = false
			} else {
				valid = valid && value.Type == gjson.String
			}
		case "messages":
			inputFound = operation != "responses"
			if !value.IsArray() {
				valid = false
			}
			value.ForEach(func(_, item gjson.Result) bool {
				valid = inspectFailoverMessage(item, operation, &content, &kind) && valid
				return true
			})
		case "system":
			valid = operation == "messages" && isAffinityTextContent(value, false) && valid
		case "tools":
			value.ForEach(func(_, tool gjson.Result) bool {
				if !tool.IsObject() {
					valid = false
					return true
				}
				switch tool.Get("type").Str {
				case "function":
					kind = failoverTools
				case "":
					valid = operation == "messages" && tool.Get("name").Str != "" && tool.Get("input_schema").IsObject() && valid
					kind = failoverTools
				default:
					inspectFailoverReferences(tool, &content)
					valid = false
				}
				return true
			})
			valid = valid && (value.Type == gjson.Null || value.IsArray())
		case "context_management":
			valid = valid && !contextSeen && operation == "messages" && claudeKeepAllContext(value)
			contextSeen = true
		case "background":
			valid = valid && !value.Bool()
		case "modalities":
			valid = valid && value.IsArray() && affinityArrayEvery(value, func(item gjson.Result) bool { return item.Str == "text" })
		case "model", "stream", "stream_options", "max_tokens", "max_completion_tokens", "max_output_tokens", "instructions",
			"metadata", "prompt_cache_key", "prompt_cache_retention", "temperature", "top_p", "top_k", "stop", "stop_sequences",
			"n", "seed", "frequency_penalty", "presence_penalty", "logit_bias", "logprobs", "top_logprobs",
			"response_format", "text", "store", "service_tier", "tool_choice", "parallel_tool_calls", "user",
			"safety_identifier", "reasoning", "reasoning_effort", "thinking", "output_config", "disable_parallel_tool_use":
		default:
			valid = false
		}
		return true
	})
	if valid && inputFound && len(content.ownerReferences) == 0 &&
		(kind != failoverTools || operation == "responses" || portableFunctionHistory(request.Get("messages"), operation)) {
		if contextSeen && (kind != failoverText || !claudeContextProfile(c, request)) {
			return content
		}
		content.kind = kind
		content.claudeKeepAll = contextSeen
	}
	return content
}

// A tool result tied to a missing call may depend on state from a prior
// provider request. Only fully represented calls and results can migrate.
func portableFunctionHistory(messages gjson.Result, operation string) bool {
	pending := make(map[string]bool)
	add := func(id string) bool {
		if id == "" || pending[id] {
			return false
		}
		pending[id] = true
		return true
	}
	finish := func(id string) bool {
		if !pending[id] {
			return false
		}
		delete(pending, id)
		return true
	}
	valid := affinityArrayEvery(messages, func(message gjson.Result) bool {
		role := message.Get("role").Str
		if operation == "chat" {
			if message.Get("function_call").Exists() || role == "function" {
				return false
			}
			if calls := message.Get("tool_calls"); calls.Exists() && calls.Type != gjson.Null {
				if role != "assistant" || !calls.IsArray() || !affinityArrayEvery(calls, func(call gjson.Result) bool {
					return call.Get("type").Str == "function" && call.Get("function.name").Str != "" &&
						call.Get("function.arguments").Type == gjson.String && add(call.Get("id").Str)
				}) {
					return false
				}
			}
			return role != "tool" || finish(message.Get("tool_call_id").Str)
		}
		content := message.Get("content")
		return !content.IsArray() || affinityArrayEvery(content, func(part gjson.Result) bool {
			switch part.Get("type").Str {
			case "tool_use":
				return role == "assistant" && part.Get("name").Str != "" && part.Get("input").IsObject() && add(part.Get("id").Str)
			case "tool_result":
				return role == "user" && finish(part.Get("tool_use_id").Str)
			default:
				return true
			}
		})
	})
	return valid && len(pending) == 0
}

func failoverValuePresent(value gjson.Result) bool {
	return value.Type != gjson.Null && (value.Type != gjson.String || strings.TrimSpace(value.Str) != "")
}

func inspectFailoverMessage(item gjson.Result, operation string, content *failoverRequestContent, kind *string) bool {
	inspectFailoverReferences(item, content)
	if !isAffinityTextMessage(item) {
		return false
	}
	valid := true
	item.ForEach(func(key, value gjson.Result) bool {
		if key.Str != strings.ToLower(key.Str) {
			valid = false
		}
		switch key.Str {
		case "role", "type", "name":
		case "content":
			if value.IsArray() {
				value.ForEach(func(_, part gjson.Result) bool {
					switch part.Get("type").Str {
					case "tool_use", "tool_result":
						*kind = failoverTools
					}
					return true
				})
			}
		case "tool_calls", "tool_call_id", "function_call":
			valid = valid && operation == "chat"
			if failoverValuePresent(value) {
				*kind = failoverTools
			}
		default:
			valid = valid && !failoverValuePresent(value)
		}
		return true
	})
	return valid
}

func inspectFailoverReferences(value gjson.Result, content *failoverRequestContent) {
	// Traverse only protocol fields, never user-controlled tool arguments,
	// metadata or JSON schemas whose properties can share these names.
	value.ForEach(func(key, item gjson.Result) bool {
		switch strings.ToLower(key.Str) {
		case "id":
			if value.Get("type").Str != "tool_use" && failoverValuePresent(item) {
				content.ownerReferences = append(content.ownerReferences, "")
			}
		case "file_id", "file_ids", "audio", "container", "container_id", "vector_store_ids", "encrypted_content":
			if failoverValuePresent(item) {
				content.ownerReferences = append(content.ownerReferences, "")
			}
		case "content":
			if item.IsArray() {
				item.ForEach(func(_, part gjson.Result) bool {
					inspectFailoverReferences(part, content)
					return true
				})
			}
		case "tool_result":
			inspectFailoverReferences(item, content)
		case "tools":
			item.ForEach(func(_, tool gjson.Result) bool {
				inspectFailoverReferences(tool, content)
				return true
			})
		case "type":
			if item.Str == "item_reference" || item.Str == "mcp_approval_response" {
				content.ownerReferences = append(content.ownerReferences, "")
			}
		}
		return true
	})
}

// The initial contract covers plain text and self-contained function tools
// via supported text adapters. Media and signed thinking history are kept on
// their original route until a verified source/target pair can be registered.
func canMigrateChannelContent(operation, kind string, source, target *model.Channel) bool {
	if kind == "" || !failoverChannelInScope(operation, target) || source != nil && !failoverChannelInScope(operation, source) {
		return false
	}
	if kind == failoverTools && source != nil && source.Type != target.Type {
		// Tool serialization across adapter families needs its own evidence.
		return slices.Contains([]int{constant.ChannelTypeOpenAI, constant.ChannelTypeNewAPI}, source.Type) &&
			slices.Contains([]int{constant.ChannelTypeOpenAI, constant.ChannelTypeNewAPI}, target.Type)
	}
	return true
}

// failoverChannelInScope is the single admission rule for a channel that
// failover may bypass, supplement from, migrate a request to, or probe.
func failoverChannelInScope(operation string, channel *model.Channel) bool {
	return channel != nil && !channel.ChannelInfo.IsMultiKey && portableTextAdapter(operation, channel.Type) &&
		failoverAdapterSettingsSafe(channel)
}

func failoverAdapterSettingsSafe(channel *model.Channel) bool {
	var settings kitdto.ChannelSettings
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return false
	}
	if channel.Setting == nil || *channel.Setting == "" {
		return true
	}
	return common.UnmarshalJsonStr(*channel.Setting, &settings) == nil && !settings.PassThroughBodyEnabled
}

func portableTextAdapter(operation string, channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeNewAPI:
		return true
	case constant.ChannelTypeAnthropic:
		return operation == "messages" || operation == "chat"
	default:
		return false
	}
}

type responseChannelOwner struct {
	ChannelID int    `json:"channel_id"`
	Revision  string `json:"revision"`
}

var responseChannelOwners = sync.OnceValue(func() *cachex.HybridCache[responseChannelOwner] {
	return cachex.NewHybridCache(cachex.HybridCacheConfig[responseChannelOwner]{
		Namespace: "new-api:native_response_owner:v1", Redis: common.RDB, RedisEnabled: func() bool { return common.RedisEnabled },
		RedisCodec: cachex.JSONCodec[responseChannelOwner]{}, RedisOpTimeout: failoverRedisTimeout,
		Memory: func() *hot.HotCache[string, responseChannelOwner] {
			return hot.NewHotCache[string, responseChannelOwner](hot.LRU, 10000).WithTTL(responseOwnerTTL).WithJanitor().Build()
		},
	})
})

func responseOwnerKey(param *RetryParam, reference string) string {
	c := param.Ctx
	token := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	user := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if token <= 0 || user <= 0 || reference == "" {
		return ""
	}
	scope := []any{user, token, param.TokenGroup, param.ModelName, reference}
	if contract := ActiveCustomerContract(c); contract != nil {
		scope = append(scope, contract.Id, contract.Version)
	}
	raw, _ := common.Marshal(scope)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func resolveResponseOwner(param *RetryParam, references []string) (responseChannelOwner, bool) {
	var owner responseChannelOwner
	for _, reference := range references {
		key := responseOwnerKey(param, reference)
		if key == "" {
			return owner, false
		}
		candidate, found, err := responseChannelOwners().Get(key)
		if err != nil || !found || owner.ChannelID != 0 && candidate != owner {
			return owner, false
		}
		owner = candidate
	}
	return owner, owner.ChannelID > 0
}

// ObserveNativeResponseOwner records only identifiers parsed from a direct
// upstream Responses result. The request outcome gates publication.
func ObserveNativeResponseOwner(c *gin.Context, id, turnState string) {
	if r := failoverRequestFrom(c); r != nil && r.operation == "responses" {
		r.responseID, r.responseTurn = id, turnState
	}
}

func (r *channelFailoverRequest) recordResponseOwner(c *gin.Context) {
	if r.operation != "responses" || r.channel == nil || r.channel.ChannelInfo.IsMultiKey || c.Writer.Status() != http.StatusOK {
		return
	}
	param := &RetryParam{Ctx: c, TokenGroup: r.tokenGroup, ModelName: r.modelName}
	owner := responseChannelOwner{ChannelID: r.channel.Id, Revision: r.scope.Revision}
	for _, entry := range []struct{ prefix, value string }{{"response:", r.responseID}, {"turn:", r.responseTurn}} {
		if entry.value == "" {
			continue
		}
		if key := responseOwnerKey(param, entry.prefix+entry.value); key != "" {
			_, _ = responseChannelOwners().CompareAndSwapWithTTL(key, nil, owner, responseOwnerTTL)
		}
	}
}

func ChannelFailoverSupportsProbe(channel *model.Channel, operation string) bool {
	return failoverChannelInScope(operation, channel)
}
