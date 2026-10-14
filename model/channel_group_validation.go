package model

import (
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// MaxChannelGroupLength counts the complete CSV list, including separators,
// in Unicode characters, matching PostgreSQL VARCHAR rather than UTF-8 bytes.
const MaxChannelGroupLength = 128

func ValidateChannelGroupLength(groups string) error {
	if utf8.RuneCountInString(groups) > MaxChannelGroupLength {
		return common.NewMessage("Channel groups must not exceed 128 characters (including commas)")
	}
	return nil
}
