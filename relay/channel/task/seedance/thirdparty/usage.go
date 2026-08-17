package thirdparty

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type terminalTokenUsageCandidate struct {
	path  string
	value int
}

type terminalTokenUsageCandidates struct {
	completion []terminalTokenUsageCandidate
	total      []terminalTokenUsageCandidate
	prompt     []terminalTokenUsageCandidate
	generic    []terminalTokenUsageCandidate
}

// TerminalTokenUsage is the outcome of scanning a successful terminal response
// for Provider usage facts. Usage/Source carry the billing-eligible completion
// value; Evidence records every validated candidate by field path for audit.
type TerminalTokenUsage struct {
	Usage    map[string]any
	Source   string
	Evidence map[string]int
}

// normalizeTerminalTokenUsage collects every numeric usage value or token-named
// value in a successful terminal response as Provider-reported usage evidence.
// It does not depend on a model/provider allowlist or a runtime trust switch.
// Only semantically verified values form billable usage: an explicit
// completion/output/generated field, total minus prompt, or a standalone total
// with no prompt field. Prompt-only, inconsistent total < prompt responses and
// other numeric usage fields stay evidence and never masquerade as completion
// tokens.
func normalizeTerminalTokenUsage(data map[string]any) TerminalTokenUsage {
	var candidates terminalTokenUsageCandidates
	collectTerminalTokenUsage(data, false, "", &candidates)

	buckets := [][]terminalTokenUsageCandidate{
		candidates.completion,
		candidates.total,
		candidates.prompt,
		candidates.generic,
	}
	evidence := map[string]int{}
	for _, values := range buckets {
		sort.Slice(values, func(i, j int) bool { return values[i].path < values[j].path })
		for _, candidate := range values {
			evidence[candidate.path] = candidate.value
		}
	}

	completion, hasCompletion := firstTerminalTokenUsage(candidates.completion)
	total, hasTotal := firstTerminalTokenUsage(candidates.total)
	prompt, hasPrompt := firstTerminalTokenUsage(candidates.prompt)

	usage := map[string]any{}
	var source string
	switch {
	case hasCompletion:
		usage["completion_tokens"] = completion.value
		if hasTotal {
			usage["total_tokens"] = total.value
		} else {
			usage["total_tokens"] = completion.value
		}
		source = completion.path
	case hasTotal && hasPrompt && total.value >= prompt.value:
		usage["completion_tokens"] = total.value - prompt.value
		usage["total_tokens"] = total.value
		source = total.path + "-" + prompt.path
	case hasTotal && !hasPrompt:
		usage["completion_tokens"] = total.value
		usage["total_tokens"] = total.value
		source = total.path
	default:
		return TerminalTokenUsage{Evidence: evidence}
	}
	return TerminalTokenUsage{Usage: usage, Source: source, Evidence: evidence}
}

func collectTerminalTokenUsage(value any, inUsage bool, path string, candidates *terminalTokenUsageCandidates) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fieldValue := object[key]
		fieldPath := key
		if path != "" {
			fieldPath = path + "." + key
		}
		normalizedKey := normalizeUsageFieldName(key)
		usageField := strings.Contains(normalizedKey, "usage")
		tokenField := strings.Contains(normalizedKey, "token")
		usageContext := inUsage || usageField

		if amount, valid := terminalTokenAmount(fieldValue); valid && (usageContext || tokenField) {
			candidate := terminalTokenUsageCandidate{path: fieldPath, value: amount}
			switch {
			case tokenField && (strings.Contains(normalizedKey, "completion") || strings.Contains(normalizedKey, "output") || strings.Contains(normalizedKey, "generated")):
				candidates.completion = append(candidates.completion, candidate)
			case tokenField && strings.Contains(normalizedKey, "total"):
				candidates.total = append(candidates.total, candidate)
			case tokenField && (strings.Contains(normalizedKey, "prompt") || strings.Contains(normalizedKey, "input")):
				candidates.prompt = append(candidates.prompt, candidate)
			default:
				candidates.generic = append(candidates.generic, candidate)
			}
		}

		switch nested := fieldValue.(type) {
		case map[string]any:
			collectTerminalTokenUsage(nested, usageContext, fieldPath, candidates)
		case []any:
			for index, item := range nested {
				collectTerminalTokenUsage(item, usageContext, fieldPath+"."+strconv.Itoa(index), candidates)
			}
		case string:
			if !usageField {
				continue
			}
			var decoded map[string]any
			if common.UnmarshalJsonStr(strings.TrimSpace(nested), &decoded) == nil {
				collectTerminalTokenUsage(decoded, true, fieldPath, candidates)
			}
		}
	}
}

func normalizeUsageFieldName(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	return strings.NewReplacer("_", "", "-", "", ".", "").Replace(field)
}

func terminalTokenAmount(value any) (int, bool) {
	switch amount := value.(type) {
	case int:
		if amount < 0 || int64(amount) > math.MaxInt32 {
			return 0, false
		}
		return amount, true
	case int64:
		if amount < 0 || amount > math.MaxInt32 {
			return 0, false
		}
		return int(amount), true
	case uint64:
		if amount > math.MaxInt32 {
			return 0, false
		}
		return int(amount), true
	case float64:
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > math.MaxInt32 || math.Trunc(amount) != amount {
			return 0, false
		}
		return int(amount), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(amount), 10, 32)
		if err != nil || parsed < 0 {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func firstTerminalTokenUsage(candidates []terminalTokenUsageCandidate) (terminalTokenUsageCandidate, bool) {
	if len(candidates) == 0 {
		return terminalTokenUsageCandidate{}, false
	}
	return candidates[0], true
}
