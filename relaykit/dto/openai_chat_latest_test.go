package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGPTChatLatestCapabilitiesMatchGPT56(t *testing.T) {
	for _, effort := range []string{"", "none", "low", "high"} {
		t.Run("effort="+effort, func(t *testing.T) {
			assert.Equal(t, GetOpenAIChatCapabilities("gpt-5.6", effort), GetOpenAIChatCapabilities("gpt-chat-latest", effort))
			request := GeneralOpenAIRequest{Model: "gpt-chat-latest", ReasoningEffort: effort}
			assert.Equal(t, "developer", request.GetSystemRoleName())
		})
	}
}

func TestGPTChatLatestRegistrationRequiresExactName(t *testing.T) {
	for _, name := range []string{"chat-latest", "gpt-chat-latest-preview", "gpt-chat-latest-2026-08-06", "GPT-chat-latest", "deployment-a"} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, OpenAIChatCapabilities{
				SupportsTemperature: true, SupportsTopP: true, SupportsLogProbs: true,
			}, GetOpenAIChatCapabilities(name, ""))
		})
	}
}
