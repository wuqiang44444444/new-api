package controller

import (
	"context"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func flushControllerEvidence(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, service.FlushRequestEvidence(ctx))
}
