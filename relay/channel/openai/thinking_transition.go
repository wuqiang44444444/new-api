package openai

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Add thinking tags to the original chunk so its metadata is emitted once.
func writeThinkingTransition(c *gin.Context, info *common.RelayInfo, response *dto.ChatCompletionsStreamResponse) error {
	state := &info.ThinkingContentInfo
	for i := range response.Choices {
		choice := &response.Choices[i]
		delta := &choice.Delta
		thought, content := delta.GetReasoningContent(), delta.GetContentString()
		_, wasOpen := state.OpenChoices[choice.Index]

		prefix := thought
		if thought != "" && !wasOpen {
			prefix = "<think>\n" + prefix
			if content == "" {
				if state.OpenChoices == nil {
					state.OpenChoices = make(map[int]struct{})
				}
				state.OpenChoices[choice.Index] = struct{}{}
			}
		}
		if content != "" && (wasOpen || thought != "") {
			prefix += "\n</think>\n"
			delete(state.OpenChoices, choice.Index)
			delta.Annotations = shiftURLCitations(delta.Annotations, utf8.RuneCountInString(prefix))
		}
		if prefix != "" {
			delta.SetContentString(prefix + content)
		}
		if prefix != "" || content == "" {
			delta.ReasoningContent = nil
			delta.Reasoning = nil
		}
	}
	return helper.ObjectData(c, response)
}

// URL citation ranges are character offsets into the delta content, so text
// prepended to that content moves every range by the same amount.
func shiftURLCitations(annotations json.RawMessage, runes int) json.RawMessage {
	parsed := gjson.ParseBytes(annotations)
	if runes == 0 || !parsed.IsArray() {
		return annotations
	}
	shifted := []byte(annotations)
	for i, annotation := range parsed.Array() {
		for _, field := range [...]string{"start_index", "end_index"} {
			value := annotation.Get("url_citation." + field)
			if value.Type != gjson.Number {
				continue
			}
			path := strconv.Itoa(i) + ".url_citation." + field
			if next, err := sjson.SetBytes(shifted, path, value.Int()+int64(runes)); err == nil {
				shifted = next
			}
		}
	}
	return shifted
}
