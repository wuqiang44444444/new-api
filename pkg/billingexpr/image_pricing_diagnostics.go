package billingexpr

import (
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

type ImagePriceWarning struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
}

// DiagnoseImagePrices reports issues only for expressions that use the
// explicit image quantity and fixed-price contract.
func DiagnoseImagePrices(expression string) []ImagePriceWarning {
	_, warnings := InspectImagePrices(expression)
	return warnings
}

// InspectImagePrices returns an explicitly importable legacy grid independently
// of any advisory about how the expression prices pixel dimensions.
func InspectImagePrices(expression string) (*ImageSizePriceTable, []ImagePriceWarning) {
	if !strings.Contains(expression, "image_count") || !strings.Contains(expression, "fixed") {
		return nil, nil
	}
	_, body := ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return nil, nil
	}
	warnings := make(map[ImagePriceWarning]bool)
	prices := make(map[string]float64)
	usesImageCount := false
	usesFixed := false
	ast.Find(tree.Node, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IdentifierNode:
			switch n.Value {
			case "image_count":
				usesImageCount = true
			case "p", "c", "len", "cr", "cc", "cc1h", "img", "img_cr", "img_o", "ai", "ao":
				warnings[ImagePriceWarning{"token_input", n.Value}] = true
			}
		case *ast.CallNode:
			if isFrozenPricingCall(n, "fixed", 1) {
				usesFixed = true
			}
			if isFrozenPricingCall(n, "param", 1) {
				if path, ok := n.Arguments[0].(*ast.StringNode); ok {
					switch path.Value {
					case "size", "quality", "model", "n", "parameters", "parameters.n", "parameters.prompt_extend":
					default:
						warnings[ImagePriceWarning{"unavailable_parameter", path.Value}] = true
					}
				}
			}
			if isFrozenPricingCall(n, "tier", 2) {
				name, named := n.Arguments[0].(*ast.StringNode)
				fixed, ok := n.Arguments[1].(*ast.CallNode)
				if named && ok && isFrozenPricingCall(fixed, "fixed", 1) {
					if price, valid := requestRuleNumber(fixed.Arguments[0]); valid {
						if previous, seen := prices[name.Value]; seen && previous != price {
							warnings[ImagePriceWarning{"conflicting_tier", name.Value}] = true
						}
						prices[name.Value] = price
					}
				}
			}
		}
		return false
	})
	if !usesImageCount || !usesFixed {
		return nil, nil
	}
	table := ReadLegacyImageSizePrices(expression)
	if table != nil {
		for _, quality := range table.Qualities {
			if table.Prices[table.SizeTiers[0]+"_"+quality] != table.Prices[table.SizeTiers[1]+"_"+quality] ||
				table.Prices[table.SizeTiers[1]+"_"+quality] != table.Prices[table.SizeTiers[2]+"_"+quality] {
				warnings[ImagePriceWarning{Code: "check_pixel_size_prices"}] = true
				break
			}
		}
	}
	out := make([]ImagePriceWarning, 0, len(warnings))
	for warning := range warnings {
		out = append(out, warning)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return out[i].Field < out[j].Field
		}
		return out[i].Code < out[j].Code
	})
	return table, out
}
