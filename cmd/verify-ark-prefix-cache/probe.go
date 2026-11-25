package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	arkResponsesURL = "https://ark.cn-beijing.volces.com/api/v3/responses"
	seedLiteModel   = "doubao-seed-2-1-lite-260915"
	arkMaxTTL       = 7 * 24 * time.Hour
	syntheticLine   = "The test inventory contains three green folders, five blank cards, and seven numbered boxes.\n"
)

var (
	labelPattern      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	endpointPattern   = regexp.MustCompile(`^ep-[a-zA-Z0-9-]{1,96}$`)
	responseIDPattern = regexp.MustCompile(`^resp_[a-zA-Z0-9_-]{1,191}$`)
)

type experimentPlan struct {
	Run      string           `json:"run"`
	Endpoint string           `json:"endpoint"`
	Steps    []experimentStep `json:"steps"`
}

type experimentStep struct {
	Name            string    `json:"name"`
	Operation       string    `json:"operation"`
	At              time.Time `json:"at"`
	Prefix          string    `json:"prefix"`
	InputRepeats    int       `json:"input_repeats"`
	MaxOutputTokens int       `json:"max_output_tokens"`
	ExpireAt        time.Time `json:"expire_at"`
}

func (plan experimentPlan) validate(now time.Time) error {
	if !labelPattern.MatchString(plan.Run) || !endpointPattern.MatchString(plan.Endpoint) || len(plan.Steps) == 0 {
		return fmt.Errorf("the plan needs a run label, an isolated ep- endpoint and at least one step")
	}
	seen := make(map[string]string, len(plan.Steps))
	deleted := make(map[string]bool)
	var previousAt time.Time
	for i, step := range plan.Steps {
		switch {
		case !labelPattern.MatchString(step.Name) || seen[step.Name] != "":
			return fmt.Errorf("step %d needs a unique label", i+1)
		case step.At.Before(now) || step.At.Before(previousAt):
			return fmt.Errorf("step %d needs a future scheduled time in nondecreasing order", i+1)
		case step.InputRepeats < 0 || step.InputRepeats > 8192 ||
			(step.MaxOutputTokens != 0 && step.MaxOutputTokens < 16) || step.MaxOutputTokens > 1024:
			return fmt.Errorf("step %d exceeds the synthetic input or output limit", i+1)
		}
		if !step.ExpireAt.IsZero() && (!step.ExpireAt.After(now) || !step.ExpireAt.After(step.At) || step.ExpireAt.Sub(step.At) > arkMaxTTL) {
			return fmt.Errorf("step %d has an invalid expiry for its scheduled time", i+1)
		}
		switch step.Operation {
		case "create":
			if step.Prefix != "" || step.InputRepeats == 0 || step.MaxOutputTokens != 0 || step.ExpireAt.IsZero() {
				return fmt.Errorf("step %d must specify synthetic input and explicit expiry for creation", i+1)
			}
		case "reuse":
			if seen[step.Prefix] != "create" || deleted[step.Prefix] {
				return fmt.Errorf("step %d must reference an earlier active prefix", i+1)
			}
		case "delete":
			if seen[step.Prefix] != "create" && seen[step.Prefix] != "reuse" {
				return fmt.Errorf("step %d must reference a previously created response", i+1)
			}
			if step.InputRepeats != 0 || step.MaxOutputTokens != 0 || !step.ExpireAt.IsZero() {
				return fmt.Errorf("step %d cannot specify input or expiry for deletion", i+1)
			}
			deleted[step.Prefix] = true
		case "control":
			if step.Prefix != "" || step.InputRepeats == 0 || !step.ExpireAt.IsZero() {
				return fmt.Errorf("step %d must specify synthetic input without caching for the control", i+1)
			}
		default:
			return fmt.Errorf("step %d has an unknown operation", i+1)
		}
		seen[step.Name] = step.Operation
		previousAt = step.At
	}
	return nil
}

type arkUsage struct {
	InputTokens  *int `json:"input_tokens,omitempty"`
	OutputTokens *int `json:"output_tokens,omitempty"`
	InputDetails *struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
}

type arkResponse struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`
	Model     string    `json:"model"`
	Status    string    `json:"status"`
	CreatedAt int64     `json:"created_at"`
	ExpireAt  int64     `json:"expire_at"`
	Deleted   bool      `json:"deleted"`
	Usage     *arkUsage `json:"usage"`
}

type observation struct {
	Run               string    `json:"run"`
	Step              string    `json:"step"`
	Operation         string    `json:"operation"`
	Prefix            string    `json:"prefix,omitempty"`
	ModelVersion      string    `json:"target_model_version"`
	EndpointDigest    string    `json:"endpoint_digest"`
	ScheduledAt       time.Time `json:"scheduled_at"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	InputRepeats      int       `json:"input_repeats,omitempty"`
	MaxOutputTokens   int       `json:"max_output_tokens,omitempty"`
	RequestedExpireAt int64     `json:"requested_expire_at,omitempty"`
	HTTPStatus        int       `json:"http_status,omitempty"`
	Outcome           string    `json:"outcome"`
	ResponseIDDigest  string    `json:"response_id_digest,omitempty"`
	ModelConfirmed    bool      `json:"provider_model_confirmed,omitempty"`
	CreatedAt         int64     `json:"provider_created_at,omitempty"`
	ExpireAt          int64     `json:"provider_expire_at,omitempty"`
	Usage             *arkUsage `json:"usage,omitempty"`
}

func runExperiment(ctx context.Context, client *http.Client, url, key string, plan experimentPlan, output io.Writer) error {
	responseIDs := make(map[string]string)
	endpointHash := sha256.Sum256([]byte(plan.Endpoint))
	for _, step := range plan.Steps {
		timer := time.NewTimer(time.Until(step.At))
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("the experiment was cancelled")
		case <-timer.C:
		}
		result := observation{
			Run: plan.Run, Step: step.Name, Operation: step.Operation, Prefix: step.Prefix,
			ModelVersion: seedLiteModel, EndpointDigest: hex.EncodeToString(endpointHash[:]),
			ScheduledAt: step.At.UTC(), StartedAt: time.Now().UTC(), InputRepeats: step.InputRepeats,
		}
		if step.Operation != "create" && step.Operation != "delete" {
			result.MaxOutputTokens = max(step.MaxOutputTokens, 16)
		}
		if !step.ExpireAt.IsZero() {
			result.RequestedExpireAt = step.ExpireAt.Unix()
		}
		response, status, outcome := sendStep(ctx, client, url, key, plan, step, result, responseIDs[step.Prefix])
		result.FinishedAt = time.Now().UTC()
		result.HTTPStatus, result.Outcome = status, outcome
		if outcome == "confirmed" {
			responseHash := sha256.Sum256([]byte(response.ID))
			result.ResponseIDDigest = hex.EncodeToString(responseHash[:])
			result.ModelConfirmed = response.Model == seedLiteModel
			result.CreatedAt, result.ExpireAt, result.Usage = response.CreatedAt, response.ExpireAt, response.Usage
			if step.Operation == "create" || step.Operation == "reuse" {
				responseIDs[step.Name] = response.ID
			}
		}
		encoded, err := common.Marshal(result)
		if err != nil {
			return fmt.Errorf("cannot encode the experiment observation")
		}
		if _, err := fmt.Fprintln(output, string(encoded)); err != nil {
			return fmt.Errorf("cannot write the experiment observation")
		}
		if outcome != "confirmed" {
			return fmt.Errorf("experiment stopped after %s at step %s; inspect the recorded outcome before any further writes", outcome, step.Name)
		}
	}
	return nil
}

func sendStep(ctx context.Context, client *http.Client, url, key string, plan experimentPlan, step experimentStep, result observation, referencedID string) (arkResponse, int, string) {
	var response arkResponse
	method := http.MethodPost
	var body io.Reader
	if step.Operation == "delete" {
		method = http.MethodDelete
		url += "/" + referencedID
	} else {
		input := strings.Repeat(syntheticLine, step.InputRepeats)
		request := map[string]any{"model": plan.Endpoint, "stream": false, "thinking": map[string]string{"type": "disabled"}}
		switch step.Operation {
		case "create":
			input = plan.Run + " " + step.Name + "\n" + input
			request["caching"] = map[string]any{"type": "enabled", "prefix": true}
			request["store"] = true
		case "reuse":
			input += "How many green folders are listed? Reply in one sentence."
			request["previous_response_id"] = referencedID
			request["caching"] = map[string]string{"type": "enabled"}
			request["store"] = true
		case "control":
			input = plan.Run + "\n" + input + "How many green folders are listed?"
			request["store"] = false
		}
		request["input"] = input
		if result.MaxOutputTokens != 0 {
			request["max_output_tokens"] = result.MaxOutputTokens
		}
		if result.RequestedExpireAt != 0 {
			if result.RequestedExpireAt <= time.Now().Unix() {
				return response, 0, "expired_before_send"
			}
			request["expire_at"] = result.RequestedExpireAt
		}
		encoded, err := common.Marshal(request)
		if err != nil {
			return response, 0, "request_encoding_failed"
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil || (referencedID == "" && (step.Operation == "reuse" || step.Operation == "delete")) {
		return response, 0, "request_construction_failed"
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	upstream, err := client.Do(req)
	if err != nil {
		return response, 0, "transport_outcome_unknown"
	}
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusOK {
		return response, upstream.StatusCode, "upstream_http_error"
	}
	if err := common.DecodeJsonBody(io.LimitReader(upstream.Body, 1<<20), &response); err != nil ||
		response.Object != "response" || !responseIDPattern.MatchString(response.ID) {
		return response, upstream.StatusCode, "unconfirmed_response"
	}
	if method == http.MethodDelete {
		if !response.Deleted || response.ID != referencedID {
			return response, upstream.StatusCode, "unconfirmed_delete"
		}
	} else {
		if response.Model != seedLiteModel && response.Model != plan.Endpoint {
			return response, upstream.StatusCode, "unexpected_provider_model"
		}
		if response.Status != "completed" || (step.Operation == "create" && (response.CreatedAt <= 0 || response.ExpireAt <= response.CreatedAt)) {
			return response, upstream.StatusCode, "unconfirmed_response"
		}
	}
	return response, upstream.StatusCode, "confirmed"
}
