package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func batchLine(customId string, model string, extra string) string {
	return `{"custom_id":"` + customId + `","method":"POST","url":"/v1/chat/completions","body":{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]` + extra + `}}`
}

func TestValidateBatchJSONLAcceptsUniformBatchAndCountsLines(t *testing.T) {
	var builder strings.Builder
	for i := 0; i < 5; i++ {
		builder.WriteString(batchLine("c"+string(rune('a'+i)), "gpt-test", `,"max_completion_tokens":128`))
		builder.WriteString("\n")
	}
	result, err := ValidateBatchJSONL(strings.NewReader(builder.String()), "")
	require.NoError(t, err)
	assert.EqualValues(t, 5, result.LineCount)
	assert.Equal(t, "gpt-test", result.Model)
	assert.NotEmpty(t, result.Checksum)
	assert.Greater(t, result.SizeBytes, int64(0))
}

func TestValidateBatchJSONLRejectsBadLinesAndLimits(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantLine  int
		wantMsg   string
		wantNoRow bool
	}{
		{
			name:     "duplicate custom_id",
			content:  batchLine("dup", "m", `,"max_tokens":8`) + "\n" + batchLine("dup", "m", `,"max_tokens":8`),
			wantLine: 2, wantMsg: "repeats custom_id",
		},
		{
			name:     "mixed models",
			content:  batchLine("a", "m1", `,"max_tokens":8`) + "\n" + batchLine("b", "m2", `,"max_tokens":8`),
			wantLine: 2, wantMsg: "single model",
		},
		{
			name:     "missing output cap",
			content:  batchLine("a", "m", ""),
			wantLine: 1, wantMsg: "output cap",
		},
		{
			name:     "bad json",
			content:  "{not json}\n",
			wantLine: 1, wantMsg: "not valid JSON",
		},
		{
			name:     "streaming is rejected",
			content:  batchLine("a", "m", `,"max_tokens":8,"stream":true`),
			wantLine: 1, wantMsg: "stream=true",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateBatchJSONL(strings.NewReader(test.content), "")
			require.Error(t, err)
			var validateErr *dto.BatchValidateError
			assert.ErrorAs(t, err, &validateErr)
			assert.Equal(t, test.wantLine, validateErr.Line)
			assert.Contains(t, validateErr.Message, test.wantMsg)
		})
	}

	t.Run("request count limit", func(t *testing.T) {
		var builder strings.Builder
		for i := 0; i < dto.MaxBatchRequestsPerFile+1; i++ {
			builder.WriteString(batchLine(fmt.Sprintf("c%07d", i), "m", `,"max_tokens":8`))
			builder.WriteString("\n")
		}
		_, err := ValidateBatchJSONL(strings.NewReader(builder.String()), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "10,000 requests per file")
	})
}

func TestValidateBatchJSONLRejectsModelMismatchAtCreateTime(t *testing.T) {
	content := batchLine("a", "model-a", `,"max_tokens":8`) + "\n"
	_, err := ValidateBatchJSONL(strings.NewReader(content), "model-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the requested model")
}

func TestConvertBatchJSONLForDeploymentRewritesOnlyTheModel(t *testing.T) {
	content := batchLine("c1", "client-model", `,"max_completion_tokens":64`) + "\n"
	converted, err := ConvertBatchJSONLForDeployment([]byte(content), "azure-deployment")
	require.NoError(t, err)
	text := string(converted.Bytes)
	assert.Contains(t, text, `"model":"azure-deployment"`)
	assert.NotContains(t, text, "client-model")
	assert.Contains(t, text, `"custom_id":"c1"`)
	assert.Contains(t, text, `"max_completion_tokens":64`)
	require.Len(t, converted.LineEstimates, 1)
	assert.Equal(t, "c1", converted.LineEstimates[0].CustomId)
	assert.EqualValues(t, 64, converted.LineEstimates[0].OutputCap)
	// The parse result describes the SOURCE file; the converted bytes carry
	// the deployment name.
	assert.Equal(t, "client-model", converted.Parse.Model)
}

func TestConvertBatchChangesOnlyTopLevelBodyModelWithWhitespace(t *testing.T) {
	line := []byte(`{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"metadata":{"model":"public"},"model" : "public","messages":[{"role":"user","content":"ok"}],"max_tokens":10}}`)
	converted, err := ConvertBatchJSONLForDeployment(line, "deployment")
	require.NoError(t, err)
	assert.Contains(t, string(converted.Bytes), `"model":"deployment"`)
	assert.Contains(t, string(converted.Bytes), `"metadata":{"model":"public"}`)
}

func TestValidateBatchRejectsDuplicateFieldsBeforeConversion(t *testing.T) {
	base := batchLine("a", "m", `,"max_tokens":1`)
	for _, line := range []string{
		strings.Replace(base, `"max_tokens":1`, `"max_tokens":1,"max_tokens":8192`, 1),
		strings.Replace(base, `"max_tokens":1`, `"max_tokens":1,"n":1,"n":129`, 1),
		strings.Replace(base, `"custom_id":"a"`, `"custom_id":"a","custom_id":"b"`, 1),
		strings.Replace(base, `"body":`, `"body":{},"body":`, 1),
		strings.Replace(base, `"max_tokens":1`, `"max_tokens":1,"temperature":0,"temperature":1`, 1),
		strings.Replace(base, `"max_tokens":1`, `"max_tokens":1,"max_\u0074okens":8192`, 1),
		strings.Replace(base, `"role":"user"`, `"role":"user","role":"system"`, 1),
	} {
		_, err := ValidateBatchJSONL(strings.NewReader(line), "")
		require.ErrorContains(t, err, "duplicate object fields")
		_, err = ConvertBatchJSONLForDeployment([]byte(line), "deployment")
		require.ErrorContains(t, err, "duplicate object fields")
	}
}
