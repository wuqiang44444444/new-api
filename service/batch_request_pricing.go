package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/tidwall/gjson"
)

// Only scalar Chat parameters can be persisted as pricing conditions. Prompts,
// tools, arbitrary metadata and credentials remain in private input objects.
var batchPricingParameters = map[string]bool{
	"model": true, "n": true, "max_tokens": true, "max_completion_tokens": true,
	"temperature": true, "top_p": true, "presence_penalty": true, "frequency_penalty": true,
	"seed": true, "service_tier": true, "reasoning_effort": true, "logprobs": true, "top_logprobs": true,
	"parallel_tool_calls": true, "store": true,
}

type batchRequestProbeGuard struct {
	invalid bool
	params  map[string]bool
}

func (v *batchRequestProbeGuard) Visit(node *ast.Node) {
	call, ok := (*node).(*ast.CallNode)
	if !ok {
		return
	}
	id, ok := call.Callee.(*ast.IdentifierNode)
	if !ok {
		return
	}
	if id.Value == "header" || id.Value == "usage" {
		v.invalid = true
		return
	}
	if id.Value != "param" {
		return
	}
	if len(call.Arguments) != 1 {
		v.invalid = true
		return
	}
	key, ok := call.Arguments[0].(*ast.StringNode)
	if !ok || !batchPricingParameters[key.Value] {
		v.invalid = true
		return
	}
	if v.params != nil {
		v.params[key.Value] = true
	}
}

func freezeBatchPricingParameters(expression string, input []byte) (map[string]json.RawMessage, error) {
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return nil, err
	}
	guard := &batchRequestProbeGuard{params: map[string]bool{}}
	ast.Walk(&tree.Node, guard)
	if guard.invalid {
		return nil, errors.New("Batch pricing request condition cannot be safely frozen")
	}
	if len(guard.params) == 0 {
		return nil, nil
	}
	result := map[string]json.RawMessage{}
	scanner := bufio.NewScanner(bytes.NewReader(input))
	scanner.Buffer(make([]byte, 64*1024), dto.MaxBatchLineBytes)
	for scanner.Scan() {
		row := gjson.ParseBytes(scanner.Bytes())
		params := map[string]json.RawMessage{}
		for key := range guard.params {
			value := row.Get("body." + key)
			if !value.Exists() {
				continue
			}
			if value.IsArray() || value.IsObject() || len(value.Raw) > 512 {
				return nil, errors.New("Batch pricing condition must be a bounded scalar")
			}
			params[key] = json.RawMessage(value.Raw)
		}
		encoded, err := common.Marshal(params)
		if err != nil {
			return nil, err
		}
		result[row.Get("custom_id").String()] = encoded
	}
	return result, scanner.Err()
}
