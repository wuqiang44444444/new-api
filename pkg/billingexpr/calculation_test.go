package billingexpr

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/expr-lang/expr/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestCalculationPreservesEvaluation(t *testing.T) {
	for _, source := range []string{
		`p * 2.5 + c * 15`,
		`(param("fast") == true ? 3 : 1) % 2 * p`,
		`p > 10 ? tier("high", max(p, c)*2) : tier("low", c)`,
		`false && int(param("missing")) > 0 ? 1 : 7`,
		`let x = [1,2,3]; sum(map(x, # * 2))`,
		`(param("missing") ?? 4) * 2`,
		`has(header("authorization"), "secret") ? 3.5 : 2.1`,
	} {
		t.Run(source, func(t *testing.T) {
			in := RequestInput{Body: []byte(`{"fast":true}`), Headers: map[string]string{"authorization": "secret"}}
			want, old, err := RunExprWithRequest(source, TokenParams{P: 20, C: 5}, in)
			require.NoError(t, err)
			in.RecordCalculation = true
			got, trace, err := RunExprWithRequest(source, TokenParams{P: 20, C: 5}, in)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, old.MatchedTier, trace.MatchedTier)
			assert.Equal(t, old.RequestRules, trace.RequestRules)
			require.NotNil(t, trace.Calculation)
			encoded, err := common.Marshal(trace.Calculation)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "secret")
			assert.NotContains(t, string(encoded), "authorization")
		})
	}
}

func TestCalculationRunIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for _, p := range []float64{1, 2, 9} {
		wg.Add(1)
		go func(p float64) {
			defer wg.Done()
			cost, trace, err := RunExprWithRequest(`p * 2`, TokenParams{P: p}, RequestInput{RecordCalculation: true})
			assert.NoError(t, err)
			assert.Equal(t, p*2, cost)
			if assert.NotNil(t, trace.Calculation) {
				assert.Equal(t, CalculationNumber(p*2), trace.Calculation.Values[len(trace.Calculation.Values)-1].Value)
			}
		}(p)
	}
	wg.Wait()
}

func TestCalculationArchiveRoundTrip(t *testing.T) {
	c := NewCalculation()
	c.Add("multiply", "quota", 80, 20, 4)
	archive := CalculationArchive{"request-a": c.Finish(80)}
	data, err := common.Marshal(archive)
	require.NoError(t, err)
	var restored CalculationArchive
	require.NoError(t, common.Unmarshal(data, &restored))
	assert.Equal(t, archive, restored)
}

func TestCalculationPreservesOptionalAndErrorSemantics(t *testing.T) {
	for _, source := range []string{`(param("obj")?.value ?? 2) * p`, `(param("obj").value + 2) * p`, `int(param("bad")) * p`} {
		in := RequestInput{Body: []byte(`{"obj":{"value":3},"bad":"invalid"}`)}
		expected, _, originalErr := RunExprWithRequest(source, TokenParams{P: 10}, in)
		in.RecordCalculation = true
		actual, _, recordedErr := RunExprWithRequest(source, TokenParams{P: 10}, in)
		if originalErr != nil {
			require.Error(t, recordedErr)
			continue
		}
		require.NoError(t, recordedErr)
		assert.Equal(t, expected, actual)
	}
}

func TestCalculationNumericAggregateOperands(t *testing.T) {
	_, trace, err := RunExprWithRequest(`sum(map([1,2,3], # * 2))`, TokenParams{}, RequestInput{RecordCalculation: true})
	require.NoError(t, err)
	data, err := common.Marshal(trace.Calculation)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[2, 4, 6]")
	assert.Contains(t, trace.Calculation.Values, CalculationValue{Node: trace.Calculation.Values[len(trace.Calculation.Values)-1].Node, Value: "12"})
}

func TestCalculationErrorDoesNotExposeRequestValue(t *testing.T) {
	_, trace, err := RunExprWithRequest(`int(header("Authorization")) * p`, TokenParams{P: 1}, RequestInput{RecordCalculation: true, Headers: map[string]string{"Authorization": "private-credential"}})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-credential")
	assert.NotContains(t, err.Error(), "Authorization")
	encoded, marshalErr := common.Marshal(trace.Calculation)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(encoded), "private-credential")
}

func TestCalculationProtectsNumericRequestConditionsThroughConversions(t *testing.T) {
	for _, source := range []string{
		`int(header("Authorization")) > 0 ? p * 2 : 0`,
		`int(param("secret")) + 1 > 0 ? p * 2 : 0`,
		`let code = param("secret"); int(code) > 0 ? p * 2 : 0`,
		`int(param("credentials").secret) > 0 ? p * 2 : 0`,
	} {
		t.Run(source, func(t *testing.T) {
			request := RequestInput{Headers: map[string]string{"Authorization": "8675309"}, Body: []byte(`{"secret":"8675309","credentials":{"secret":8675309}}`)}
			want, _, err := RunExprWithRequest(source, TokenParams{P: 20}, request)
			require.NoError(t, err)
			request.RecordCalculation = true
			got, trace, err := RunExprWithRequest(source, TokenParams{P: 20}, request)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			encoded, err := common.Marshal(trace.Calculation)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "8675309")
			assert.NotContains(t, string(encoded), "8675310")
			assert.NotContains(t, string(encoded), "8.675309")
			assert.Contains(t, string(encoded), `"value":"true"`)
			assert.Contains(t, string(encoded), `"value":"40"`, "the selected price arithmetic remains inspectable")
		})
	}
}

func TestCalculationErrorsKeepSafePhaseAndLocation(t *testing.T) {
	for _, tc := range []struct{ source, phase string }{
		{`param("secret") +`, "compile"},
		{`int(header("Authorization"))`, "run"},
	} {
		_, _, err := RunExprWithRequest(tc.source, TokenParams{}, RequestInput{RecordCalculation: true, Headers: map[string]string{"Authorization": "private-value"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.phase)
		assert.Contains(t, err.Error(), "line 1")
		assert.NotContains(t, err.Error(), "private-value")
		assert.NotContains(t, err.Error(), "secret")
		var sourceError *file.Error
		assert.ErrorAs(t, err, &sourceError, "internal callers retain the original error for controlled diagnostics")
	}
}
