package model

import (
	"errors"
	"strings"
)

func validateOperationalAuditNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if note == "" || len(note) > 1000 || containsControlCharacter(note) {
		return "", errors.New("operational audit note is invalid")
	}
	lower := strings.ToLower(note)
	for _, sensitive := range []string{
		"http://",
		"https://",
		"bearer ",
		"authorization:",
		"api_key",
		"api-key",
		"cookie:",
	} {
		if strings.Contains(lower, sensitive) {
			return "", errors.New("operational audit note contains sensitive material")
		}
	}
	return note, nil
}
