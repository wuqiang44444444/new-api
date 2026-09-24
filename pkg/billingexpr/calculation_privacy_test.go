package billingexpr

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculationProtectsDerivedRequestValues(t *testing.T) {
	for _, source := range []string{
		`reduce([1,2], #acc + int(header("Authorization")), 0) > 0 ? p * 2 : 0`,
		`reduce([1,2], #acc + #, int(header("Authorization"))) > 0 ? p * 2 : 0`,
		`reduce([int(header("Authorization")),2], #acc + #) > 0 ? p * 2 : 0`,
		`reduce([1,2], #acc + sum(map([1], # + int(header("Authorization")))), 0) > 0 ? p * 2 : 0`,
		`reduce([1,2], #acc + reduce([1,2], #acc + #, int(header("Authorization"))), 0) > 0 ? p * 2 : 0`,
		`int(param("apiKey")) > 0 ? p * 2 : 0`,
		`let read = param; int(read("apiKey")) > 0 ? p * 2 : 0`,
		`int($env.param("apiKey")) > 0 ? p * 2 : 0`,
		`param("access-token") > 0 ? p * 2 : 0`,
		`sum(values(param("credentials"))) > 0 ? p * 2 : 0`,
		`sum(values(param("payload"))) > 0 ? p * 2 : 0`,
		`int(param("payload.*")) > 0 ? p * 2 : 0`,
		`sum(param("payload.@values")) > 0 ? p * 2 : 0`,
		`let data = param("payload"); sum(values(data)) > 0 ? p * 2 : 0`,
		`int(param("value")) > 0 ? p * 2 : 0`,
		`sum(map(param("values"), int(#))) > 0 ? p * 2 : 0`,
		`reduce([1,2], #acc + int(param("value")), 0) > 0 ? p * 2 : 0`,
	} {
		t.Run(source, func(t *testing.T) {
			// Synthetic numeric credentials expose conversion and container leaks
			// that tests containing only nonnumeric strings cannot detect.
			request := RequestInput{
				Headers: map[string]string{"Authorization": "8675309"},
				Body:    []byte(`{"apiKey":8675309,"access-token":8675309,"credentials":{"secret":8675309},"payload":{"secret":8675309},"value":"8675309","values":["8675309","8675309"]}`),
			}
			want, _, err := RunExprWithRequest(source, TokenParams{P: 20}, request)
			require.NoError(t, err)
			request.RecordCalculation = true
			got, trace, err := RunExprWithRequest(source, TokenParams{P: 20}, request)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, float64(40), got)
			require.NotNil(t, trace.Calculation)
			encoded, err := common.Marshal(trace.Calculation)
			require.NoError(t, err)
			var stored Calculation
			require.NoError(t, common.Unmarshal(encoded, &stored))
			for _, value := range stored.Values {
				// All public numeric operands in these expressions are small.
				assert.Contains(t, []string{"protected", "0", "1", "2", "3", "20", "40", "true", "[1]", "[1, 2]"}, value.Value)
			}
			assert.Contains(t, string(encoded), `"value":"true"`)
			assert.Contains(t, string(encoded), `"value":"40"`, "the selected price arithmetic remains inspectable")
		})
	}
}

func TestCalculationRuntimePrivacyDoesNotChangeCachedPrograms(t *testing.T) {
	const source = `sum(map(param("values"), int(#)))`
	for _, test := range []struct {
		name string
		body string
		hide bool
	}{
		{"numbers_before", `{"values":[11,22]}`, false},
		{"strings", `{"values":["11","22"]}`, true},
		{"numbers_after", `{"values":[11,22]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, trace, err := RunExprWithRequest(source, TokenParams{}, RequestInput{Body: []byte(test.body), RecordCalculation: true})
			require.NoError(t, err)
			assert.Equal(t, float64(33), got)
			require.NotEmpty(t, trace.Calculation.Values)
			last := trace.Calculation.Values[len(trace.Calculation.Values)-1].Value
			if test.hide {
				assert.Equal(t, "protected", last)
			} else {
				assert.Equal(t, "33", last)
			}
		})
	}
}

func TestCalculationProtectsPartialTraceOnEvaluationError(t *testing.T) {
	const source = `sum(values(param("payload"))) + int(param("invalid"))`
	request := RequestInput{Body: []byte(`{"payload":{"secret":8675309},"invalid":"not-a-number"}`), RecordCalculation: true}
	_, trace, err := RunExprWithRequest(source, TokenParams{}, request)
	require.Error(t, err)
	require.NotNil(t, trace.Calculation)
	encoded, marshalErr := common.Marshal(trace.Calculation)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(encoded), "8675309")
	assert.NotContains(t, string(encoded), "8.675309")
	assert.NotContains(t, err.Error(), "not-a-number")
}

func TestCalculationPreservesPublicPredicateOperands(t *testing.T) {
	const source = `reduce([1,2], #acc + sum(map([3,4], # + #index)), 0)`
	want, _, err := RunExprWithRequest(source, TokenParams{}, RequestInput{})
	require.NoError(t, err)
	got, trace, err := RunExprWithRequest(source, TokenParams{}, RequestInput{RecordCalculation: true})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, float64(16), got)
	assert.Equal(t, "16", trace.Calculation.Values[len(trace.Calculation.Values)-1].Value)
	for _, value := range trace.Calculation.Values {
		assert.NotEqual(t, "protected", value.Value)
	}
}

func TestCalculationPreservesExactNumericRequestPaths(t *testing.T) {
	const source = `param("input.count") + param("input.ratios.0")`
	got, trace, err := RunExprWithRequest(source, TokenParams{}, RequestInput{
		Body: []byte(`{"input":{"count":11,"ratios":[22]}}`), RecordCalculation: true,
	})
	require.NoError(t, err)
	assert.Equal(t, float64(33), got)
	assert.Equal(t, "33", trace.Calculation.Values[len(trace.Calculation.Values)-1].Value)
	for _, value := range trace.Calculation.Values {
		assert.NotEqual(t, "protected", value.Value)
	}
}
