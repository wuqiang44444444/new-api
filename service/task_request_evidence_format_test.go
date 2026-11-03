package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceJSONFormattingPreservesSourceTokensAndRejectsIncompleteOrExpandedViews(t *testing.T) {
	for _, tc := range []struct {
		name, body, status, expected string
		limited                      bool
	}{
		{name: "numeric lexemes and duplicate keys", body: `{"id":9007199254740993,"id":18446744073709551615,"amount":1.00,"s":"[\\\"{}]"}`, status: "formatted", expected: "{\n  \"id\": 9007199254740993,\n  \"id\": 18446744073709551615,\n  \"amount\": 1.00,\n  \"s\": \"[\\\\\\\"{}]\"\n}"},
		{name: "malformed", body: `{"unclosed":`, status: "invalid_json"},
		{name: "SSE", body: "data: {}\n\n", status: "invalid_json"},
		{name: "limited preview may look valid", body: `{}`, limited: true, status: "display_limited"},
		{name: "deep expansion", body: strings.Repeat("[", 1000) + "0" + strings.Repeat("]", 1000), status: "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := &EvidenceEventContent{Body: []byte(tc.body), DisplayLimited: tc.limited}
			FormatEvidenceContentJSON(content)
			assert.Equal(t, tc.status, content.JSONStatus)
			assert.Equal(t, tc.body, string(content.Body), "formatting never replaces original bytes")
			if tc.status == "formatted" {
				require.NotNil(t, content.FormattedText)
				assert.Equal(t, tc.expected, *content.FormattedText)
			} else {
				assert.Nil(t, content.FormattedText)
			}
		})
	}
}

func TestEvidenceJSONMessageFormattingSharesOnePreviewBudget(t *testing.T) {
	body := `{"items":[` + strings.Repeat(`0,`, 6000) + `0]}`
	content := &EvidenceEventContent{Messages: []EvidenceRealtimeMessage{{Text: &body}, {Text: &body}}}
	FormatEvidenceContentJSON(content)
	assert.Equal(t, "formatted", content.Messages[0].JSONStatus)
	assert.Equal(t, "too_large", content.Messages[1].JSONStatus)
	assert.Nil(t, content.Messages[1].FormattedText)
}
