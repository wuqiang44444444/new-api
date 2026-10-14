package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelGroupLengthBoundary(t *testing.T) {
	for _, test := range []struct {
		name   string
		groups string
		valid  bool
	}{
		{"empty patch", "", true},
		{"128 including separator", strings.Repeat("a", 126) + ",b", true},
		{"129 including separator", strings.Repeat("a", 127) + ",b", false},
		{"unicode character boundary", strings.Repeat("组", 128), true},
		{"unicode overflow", strings.Repeat("组", 129), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateChannelGroupLength(test.groups)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "128")
			}
		})
	}
}

func TestTagGroupEditRejectsOversizedInputBeforeWriting(t *testing.T) {
	// The guard must run before any database lookup or bulk update, including
	// SQLite, which would otherwise accept an oversized VARCHAR value.
	groups := strings.Repeat("x", 129)
	require.Error(t, EditChannelByTagWithActor("tag", nil, nil, nil, &groups, nil, nil, nil, nil, 0))
}
