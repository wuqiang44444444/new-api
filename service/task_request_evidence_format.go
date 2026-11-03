package service

import "github.com/QuantumNous/new-api/common"

// Formatting is explicitly requested by a viewer, never by capture or directory reads.
// The total formatted output shares the 64 KiB preview budget, including WS messages.
func FormatEvidenceContentJSON(content *EvidenceEventContent) {
	budget := evidenceViewBytes
	if content.Messages == nil {
		content.FormattedText, content.JSONStatus = formatEvidenceJSON(content.Body, content.DisplayLimited, &budget)
		return
	}
	for i := range content.Messages {
		frame := &content.Messages[i]
		if frame.Text == nil {
			continue
		}
		frame.FormattedText, frame.JSONStatus = formatEvidenceJSON([]byte(*frame.Text), frame.DisplayLimited, &budget)
	}
}

func formatEvidenceJSON(body []byte, limited bool, budget *int) (*string, string) {
	if limited {
		return nil, "display_limited"
	}
	if !EvidenceContentIsText(body) {
		return nil, "not_text"
	}
	// Conservative output bound before json.Indent: deep nesting must not expand
	// a small input into an unbounded allocation. This scan only runs on a read.
	size, depth := len(body), 0
	quoted, escaped := false, false
	for _, ch := range body {
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			size += 1 + 2*depth
		case '}', ']':
			depth--
			if depth < 0 {
				return nil, "invalid_json"
			}
			size += 1 + 2*depth
		case ',':
			size += 1 + 2*depth
		case ':':
			size++
		}
		if size > *budget {
			return nil, "too_large"
		}
	}
	if size > *budget {
		return nil, "too_large"
	}
	formatted, err := common.IndentJson(body)
	if err != nil {
		return nil, "invalid_json"
	}
	*budget -= len(formatted)
	text := string(formatted)
	return &text, "formatted"
}
