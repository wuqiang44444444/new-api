package geminichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseGeminiChat2OpenAIKeepsThoughtPartsPerCandidate(t *testing.T) {
	response := ResponseGeminiChat2OpenAI("response", 1, &dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{Index: 0, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{
				{Thought: true, Text: "first"},
				{Text: "answer"},
				{Thought: true, Text: " "},
				{Thought: true, Text: "second"},
				{Thought: true},
			}}},
			{Index: 1, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "without thought"}}}},
			{Index: 2, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Thought: true}}}},
		},
	})

	require.Len(t, response.Choices, 3)
	first := response.Choices[0].Message
	require.NotNil(t, first.ReasoningContent)
	assert.Equal(t, "first second", *first.ReasoningContent)
	assert.Equal(t, "answer", first.StringContent())
	assert.Nil(t, response.Choices[1].Message.ReasoningContent)
	assert.Equal(t, "without thought", response.Choices[1].Message.StringContent())
	require.NotNil(t, response.Choices[2].Message.ReasoningContent)
	assert.Empty(t, *response.Choices[2].Message.ReasoningContent)
}

func TestStreamResponseGeminiChat2OpenAISeparatesMixedPartsPerCandidate(t *testing.T) {
	stop := "STOP"
	upstream := &dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{Index: 2, FinishReason: &stop, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{
				{Thought: true, Text: "first thought\n\n"},
				{Text: "first answer"},
				{Thought: true},
				{Thought: true, Text: "second thought"},
				{Text: "second answer"},
			}}},
			{Index: 1, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Thought: true, Text: "thought only"}}}},
			{Index: 0, Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "answer only"}}}},
		},
	}
	nonStream := ResponseGeminiChat2OpenAI("response", 1, upstream)
	response, isStop := StreamResponseGeminiChat2OpenAI(upstream)

	assert.True(t, isStop)
	require.Len(t, response.Choices, 3)
	assert.Equal(t, 2, response.Choices[0].Index)
	assert.Equal(t, "first thought\n\nsecond thought", response.Choices[0].Delta.GetReasoningContent())
	assert.Equal(t, *nonStream.Choices[0].Message.ReasoningContent, response.Choices[0].Delta.GetReasoningContent())
	assert.Equal(t, "first answer\nsecond answer", response.Choices[0].Delta.GetContentString())
	assert.Equal(t, "thought only", response.Choices[1].Delta.GetReasoningContent())
	assert.Nil(t, response.Choices[1].Delta.Content)
	assert.Equal(t, "answer only", response.Choices[2].Delta.GetContentString())
	assert.Nil(t, response.Choices[2].Delta.ReasoningContent)
}
