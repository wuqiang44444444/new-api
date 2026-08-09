package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveModelMappingPreservesOrdinaryLiteralSemantics(t *testing.T) {
	tests := []struct {
		name           string
		mapping        map[string]string
		wantModel      string
		wantApplied    bool
		wantCycleError bool
	}{
		{name: "identity", mapping: nil, wantModel: "customer-model"},
		{name: "chain", mapping: map[string]string{"customer-model": "alias", "alias": "provider-model"}, wantModel: "provider-model", wantApplied: true},
		{name: "literal whitespace", mapping: map[string]string{"customer-model": " provider-model "}, wantModel: " provider-model ", wantApplied: true},
		{name: "self mapping", mapping: map[string]string{"customer-model": "customer-model"}, wantModel: "customer-model", wantApplied: true},
		{name: "cycle", mapping: map[string]string{"customer-model": "alias", "alias": "customer-model"}, wantCycleError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelName, applied, err := ResolveModelMapping("customer-model", test.mapping)
			if test.wantCycleError {
				require.ErrorIs(t, err, ErrModelMappingCycle)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantModel, modelName)
			assert.Equal(t, test.wantApplied, applied)
		})
	}
}
