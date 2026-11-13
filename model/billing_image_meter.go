package model

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// BillingImageTotals keeps gateway pricing quantities separate from delivery
// counts and from native channel tests.
type BillingImageTotals struct {
	BillableImages               int64 `json:"billable_images,omitempty"`
	BillableImageMissingRows     int64 `json:"billable_image_missing_rows,omitempty"`
	TestBillableImages           int64 `json:"test_billable_images,omitempty"`
	TestBillableImageMissingRows int64 `json:"test_billable_image_missing_rows,omitempty"`
}

func (m *BillingImageTotals) add(other BillingImageTotals) {
	m.BillableImages += other.BillableImages
	m.BillableImageMissingRows += other.BillableImageMissingRows
	m.TestBillableImages += other.TestBillableImages
	m.TestBillableImageMissingRows += other.TestBillableImageMissingRows
}

func (m *BillingImageTotals) observe(count *int64, applicable, test bool) {
	if !applicable {
		return
	}
	if test {
		if count == nil {
			m.TestBillableImageMissingRows++
		} else {
			m.TestBillableImages += *count
		}
		return
	}
	if count == nil {
		m.BillableImageMissingRows++
	} else {
		m.BillableImages += *count
	}
}

// frozenBillingImage reads the settled meter. A create reservation and the
// completed-image count are independent of the expression's quantity.
func frozenBillingImage(log billingReconciliationLog, parsed parsedBillingReconciliationLog, facts customerExportFacts) (*int64, bool) {
	if parsed.unavailable || (log.Type != LogTypeConsume && log.Type != LogTypeRefund) {
		return nil, false
	}
	task := parsed.isTask || parsed.taskID != "" || parsed.taskBillingEvent != ""
	raw := facts.raw("image_count")
	if task {
		_, actual := facts.other["actual_quota"]
		_, pre := facts.other["pre_consumed_quota"]
		// Native image completion deliberately uses "create" in the legacy
		// log classification; its durable delivery identity retains "complete".
		complete := log.Type == LogTypeConsume && (parsed.taskBillingEvent == "complete" ||
			(strings.HasPrefix(log.RequestId, "task-billing:") && strings.HasSuffix(log.RequestId, ":complete")))
		if !complete && (parsed.taskID == "" || !isTaskBillingAdjustment(parsed.taskBillingEvent, actual, pre)) {
			return nil, false
		}
		var units map[string]string
		if common.Unmarshal(facts.raw("usage_units"), &units) == nil && units["image_count"] == "count" {
			expression, err := base64.StdEncoding.DecodeString(facts.string("expr_b64"))
			if err != nil {
				return nil, false
			}
			if !billingexpr.UsedUsageKeys(string(expression))["image_count"] {
				return nil, false
			}
			// The final calculation, rather than the creation snapshot's
			// public quantity, supplies the typed task meter.
			var calculation struct {
				UsageFacts  map[string]json.RawMessage `json:"usage_facts"`
				MatchedTier string                     `json:"matched_tier"`
			}
			if common.Unmarshal(facts.raw("billing_calculation"), &calculation) != nil ||
				calculation.MatchedTier == "" || calculation.MatchedTier != facts.string("matched_tier") {
				return nil, true
			}
			raw = calculation.UsageFacts["image_count"]
			return boundedBillingImageCount(raw), true
		}
		// Older native async logs can prove the mode from the settled tier,
		// but only the calculation can prove the quantity used for pricing.
		if complete {
			var calculation billingexpr.Calculation
			if common.Unmarshal(facts.raw("billing_calculation"), &calculation) == nil {
				switch calculation.BillingUnit {
				case billingexpr.BillingUnitRequest:
					if calculation.ImageCount == nil {
						return nil, false
					}
					if *calculation.ImageCount <= 0 || *calculation.ImageCount > dto.MaxImageN {
						return nil, true
					}
					count := int64(*calculation.ImageCount)
					return &count, true
				case "":
					expression, err := base64.StdEncoding.DecodeString(facts.string("expr_b64"))
					return nil, err == nil && customerUsageTierMetersForTask(string(expression), calculation.MatchedTier) == usageFixedImage
				}
			}
		}
		return nil, false
	}
	if log.Type != LogTypeConsume || facts.string("billing_unit") != string(billingexpr.BillingUnitRequest) {
		return nil, false
	}
	if frozenFixedRequestMeter(facts) != customerUsagePerImage {
		return nil, false
	}
	if parsed.testPricing != nil && parsed.testPricing.Status != upstreamTestPricingStatusSettled {
		return nil, true
	}
	return boundedBillingImageCount(raw), true
}

func frozenFixedRequestMeter(facts customerExportFacts) string {
	expression, err := base64.StdEncoding.DecodeString(facts.string("expr_b64"))
	if err != nil {
		return BillingReconciliationModeUnknown
	}
	return customerUsageFixedTierMeter(string(expression), facts.string("matched_tier"))
}

func boundedBillingImageCount(raw json.RawMessage) *int64 {
	count, ok := billingBreakdownNonNegativeInt(raw)
	if !ok || count <= 0 || count > int64(dto.MaxImageN) {
		return nil
	}
	return &count
}
