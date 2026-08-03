package dto

import "strings"

// LinkImplementationRef identifies the immutable Link implementation selected
// by an individual channel. The implementation registry lives in the root
// module so relaykit remains independently buildable.
type LinkImplementationRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (ref LinkImplementationRef) Empty() bool {
	return strings.TrimSpace(ref.ID) == "" && strings.TrimSpace(ref.Version) == ""
}

func (ref LinkImplementationRef) Valid() bool {
	return strings.TrimSpace(ref.ID) != "" && strings.TrimSpace(ref.Version) != ""
}
