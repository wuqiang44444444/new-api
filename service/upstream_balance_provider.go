package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

type upstreamBalanceError string

func (e upstreamBalanceError) Error() string { return string(e) }

var upstreamBalanceNumber = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,2})?$`)
var upstreamBalanceUnit = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,23}$`)

// Decimal strings avoid rounding provider money through float64. Exponent and
// response bounds also prevent untrusted values causing huge decimal allocation.
func parseUpstreamBalanceNumber(raw json.RawMessage) (decimal.Decimal, error) {
	value := strings.TrimSpace(string(raw))
	if strings.HasPrefix(value, "\"") {
		if err := common.Unmarshal(raw, &value); err != nil {
			return decimal.Zero, upstreamBalanceError("invalid_response")
		}
	}
	if len(value) > 96 || !upstreamBalanceNumber.MatchString(value) {
		return decimal.Zero, upstreamBalanceError("invalid_response")
	}
	number, err := decimal.NewFromString(value)
	if err != nil || number.Exponent() < -30 || number.Exponent() > 30 {
		return decimal.Zero, upstreamBalanceError("invalid_response")
	}
	return number, nil
}

// Never return URLs, transport errors, provider messages or raw response bodies.
func getUpstreamBalanceJSON(ctx context.Context, client *http.Client, target, authorization string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return upstreamBalanceError("invalid_connection")
	}
	req.Header.Set("Accept", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
			return upstreamBalanceError("timeout")
		}
		return upstreamBalanceError("network_error")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return upstreamBalanceError("authentication_failed")
	case http.StatusNotFound:
		return upstreamBalanceError("endpoint_unavailable")
	case http.StatusTooManyRequests:
		return upstreamBalanceError("rate_limited")
	}
	if response.StatusCode != http.StatusOK {
		return upstreamBalanceError("upstream_error")
	}
	const limit = 256 * 1024
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(body) > limit {
		return upstreamBalanceError("invalid_response")
	}
	var envelope struct {
		Success *bool           `json:"success"`
		Error   json.RawMessage `json:"error"`
	}
	if common.Unmarshal(body, &envelope) != nil || envelope.Success != nil && !*envelope.Success || len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return upstreamBalanceError("invalid_response")
	}
	if common.Unmarshal(body, output) != nil {
		return upstreamBalanceError("invalid_response")
	}
	return nil
}

func queryUpstreamBalanceProvider(ctx context.Context, client *http.Client, plan upstreamBalancePlan, key string) UpstreamBalanceResult {
	result, err := readUpstreamBalance(ctx, client, plan, key)
	if err != nil {
		return UpstreamBalanceResult{Status: "error", Reason: err.Error()}
	}
	return result
}

func readUpstreamBalance(ctx context.Context, client *http.Client, plan upstreamBalancePlan, key string) (UpstreamBalanceResult, error) {
	result := UpstreamBalanceResult{Status: "ok", Scope: "upstream_balance"}
	auth := "Bearer " + key
	switch plan.kind {
	case "leone":
		var body struct {
			Code *int `json:"code"`
			Data struct {
				Balance  json.RawMessage `json:"balance"`
				Currency string          `json:"currency"`
			} `json:"data"`
		}
		if err := getUpstreamBalanceJSON(ctx, client, plan.origin+"/api/v2/open/balance", auth, &body); err != nil {
			return result, err
		}
		if body.Code == nil || *body.Code != 0 || !upstreamBalanceUnit.MatchString(body.Data.Currency) {
			return result, upstreamBalanceError("invalid_response")
		}
		amount, err := parseUpstreamBalanceNumber(body.Data.Balance)
		if err != nil {
			return result, err
		}
		result.Amounts = []UpstreamBalanceAmount{{Amount: amount.String(), Unit: body.Data.Currency}}
	case "vidu":
		var body struct {
			Remains []struct {
				Type         string          `json:"type"`
				CreditRemain json.RawMessage `json:"credit_remain"`
			} `json:"remains"`
		}
		if err := getUpstreamBalanceJSON(ctx, client, plan.origin+"/ent/v2/credits", "Token "+key, &body); err != nil {
			return result, err
		}
		if len(body.Remains) == 0 {
			return result, upstreamBalanceError("invalid_response")
		}
		result.Scope = "credits"
		categories := make(map[string]bool)
		for _, entry := range body.Remains {
			amount, err := parseUpstreamBalanceNumber(entry.CreditRemain)
			if err != nil || !upstreamBalanceUnit.MatchString(entry.Type) || categories[entry.Type] {
				return result, upstreamBalanceError("invalid_response")
			}
			categories[entry.Type] = true
			result.Amounts = append(result.Amounts, UpstreamBalanceAmount{Amount: amount.String(), Unit: "credits", Category: entry.Type})
		}
	case "newapi", "qhaigc":
		var subscription struct {
			Object    string          `json:"object"`
			HardLimit json.RawMessage `json:"hard_limit_usd"`
			Balance   json.RawMessage `json:"balance"`
		}
		if err := getUpstreamBalanceJSON(ctx, client, plan.origin+"/dashboard/billing/subscription", auth, &subscription); err != nil {
			return result, err
		}
		if subscription.Object != "billing_subscription" {
			return result, upstreamBalanceError("invalid_response")
		}
		limit, err := parseUpstreamBalanceNumber(subscription.HardLimit)
		if err != nil {
			return result, err
		}
		// This sentinel is meaningful only inside these explicitly verified NewAPI
		// deployments. It is never evidence of unlimited account funds.
		if limit.Equal(decimal.NewFromInt(100000000)) {
			result.Status, result.Reason, result.Scope = "unavailable", "account_balance_unavailable", "unconfirmed"
			if plan.tokenUsage {
				var token struct {
					Code *bool `json:"code"`
					Data struct {
						Object    string `json:"object"`
						Unlimited *bool  `json:"unlimited_quota"`
					} `json:"data"`
				}
				if err := getUpstreamBalanceJSON(ctx, client, plan.origin+"/api/usage/token/", auth, &token); err != nil {
					return result, err
				}
				if token.Code == nil || !*token.Code || token.Data.Object != "token_usage" || token.Data.Unlimited == nil {
					return result, upstreamBalanceError("invalid_response")
				}
				if *token.Data.Unlimited {
					result.Status, result.Reason = "unlimited", "unlimited_key"
				}
			}
			return result, nil
		}
		var amount decimal.Decimal
		if plan.kind == "qhaigc" {
			amount, err = parseUpstreamBalanceNumber(subscription.Balance)
			if err != nil {
				return result, err
			}
		} else {
			var usage struct {
				Object string          `json:"object"`
				Total  json.RawMessage `json:"total_usage"`
			}
			if err := getUpstreamBalanceJSON(ctx, client, plan.origin+"/dashboard/billing/usage", auth, &usage); err != nil {
				return result, err
			}
			if usage.Object != "list" {
				return result, upstreamBalanceError("invalid_response")
			}
			used, err := parseUpstreamBalanceNumber(usage.Total)
			if err != nil {
				return result, err
			}
			amount = limit.Sub(used.Shift(-2))
		}
		unit := ""
		if plan.kind == "newapi" {
			var status struct {
				Success *bool `json:"success"`
				Data    struct {
					Display  string `json:"quota_display_type"`
					Currency string `json:"currency_code"`
				} `json:"data"`
			}
			// Currency metadata is public. Never attach credentials to this request.
			if getUpstreamBalanceJSON(ctx, client, plan.origin+"/api/status", "", &status) == nil && status.Success != nil && *status.Success {
				if status.Data.Display == "USD" || status.Data.Display == "CNY" {
					unit = status.Data.Display
				} else if upstreamBalanceUnit.MatchString(status.Data.Currency) {
					unit = status.Data.Currency
				}
			}
		}
		result.Scope = "unconfirmed"
		result.Amounts = []UpstreamBalanceAmount{{Amount: amount.String(), Unit: unit}}
	default:
		return result, upstreamBalanceError("not_connected")
	}
	return result, nil
}
