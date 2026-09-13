package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaClampJSONPreservesFiniteAndNonFiniteDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     float64
		jsonValue string
	}{
		{"finite", 3000000000, `3000000000`},
		{"positive infinity", math.Inf(1), `"+Inf"`},
		{"negative infinity", math.Inf(-1), `"-Inf"`},
		{"not a number", math.NaN(), `"NaN"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := QuotaClamp{Op: "QuotaRound", Kind: QuotaClampOverflow, Original: tc.value, Clamped: MaxQuota}
			encoded, err := Marshal(original)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"original":`+tc.jsonValue)
			audit, err := Marshal(original.AuditMap())
			require.NoError(t, err)
			assert.JSONEq(t, string(encoded), string(audit))
			var restored QuotaClamp
			require.NoError(t, Unmarshal(encoded, &restored))
			assert.Equal(t, original.Op, restored.Op)
			assert.Equal(t, original.Kind, restored.Kind)
			assert.Equal(t, original.Clamped, restored.Clamped)
			if math.IsNaN(tc.value) {
				assert.True(t, math.IsNaN(restored.Original))
			} else {
				assert.Equal(t, tc.value, restored.Original)
			}
		})
	}
	var invalid QuotaClamp
	assert.Error(t, UnmarshalJsonStr(`{"original":"invalid"}`, &invalid))
}
