package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestVideoRefundProofBindsObjectVersionAndReason(t *testing.T) {
	input := VideoRefundContext{Kind: "task", ID: 42, Version: strings.Repeat("a", 64), Note: "missing result"}
	encoded, err := common.Marshal(input)
	require.NoError(t, err)
	original, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeVideoRefund, Context: encoded})
	require.NoError(t, err)
	for _, mutate := range []func(*VideoRefundContext){func(v *VideoRefundContext) { v.ID++ }, func(v *VideoRefundContext) { v.Kind = "attempt" }, func(v *VideoRefundContext) { v.Version = strings.Repeat("b", 64) }, func(v *VideoRefundContext) { v.Note = "another reason" }} {
		copy := input
		mutate(&copy)
		data, err := common.Marshal(copy)
		require.NoError(t, err)
		binding, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeVideoRefund, Context: data})
		require.NoError(t, err)
		assert.NotEqual(t, original.ContextHash, binding.ContextHash)
	}
	_, err = BindVerificationOperation(VerificationOperation{Scope: VerificationScopeVideoRefund})
	require.Error(t, err)
}
