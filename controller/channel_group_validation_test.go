package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestChannelSaveRejectsOversizedGroups(t *testing.T) {
	for _, isAdd := range []bool{true, false} {
		require.ErrorContains(t, validateChannel(&model.Channel{Group: strings.Repeat("x", 129)}, isAdd), "128 characters")
	}
}
