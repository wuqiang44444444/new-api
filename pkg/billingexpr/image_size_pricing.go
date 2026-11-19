package billingexpr

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// ImageSizePriceTable is an editable projection of a recognized expression.
// Only the generated expression is persisted. Keys retain the frozen tier names.
type ImageSizePriceTable struct {
	Profile              string             `json:"profile"`
	Prices               map[string]float64 `json:"prices"`
	Qualities            []string           `json:"qualities"`
	DefaultQuality       string             `json:"default_quality"`
	SizeTiers            []string           `json:"size_tiers"`
	UnmatchedUsesQuality bool               `json:"unmatched_uses_quality"`
}

type imagePriceShape struct {
	id             string
	qualities      []string
	defaultQuality string
	resolution     bool
	squareAlias    bool
}

// These are supported price-tree structures. No model identifiers participate.
var imagePriceShapes = []imagePriceShape{
	{"medium_three", []string{"low", "high"}, "medium", false, true},
	{"medium_five", []string{"low", "high", "xhigh", "max"}, "medium", false, false},
	{"high_five", []string{"low", "medium", "xhigh", "max"}, "high", true, false},
	{"size_only", nil, "default", false, false},
}

func (s imagePriceShape) tiers() []string {
	if s.resolution {
		return []string{"1k", "2k", "4k"}
	}
	return []string{"1K", "2K", "4K"}
}

func (s imagePriceShape) expression(prices map[string]float64, pixels bool) string {
	tiers := s.tiers()
	sizeCond := func(tier string) string {
		k := strings.ToUpper(tier)
		literal := fmt.Sprintf(`param("size") == %q`, k)
		if pixels {
			const size = `param("size")`
			const valid = `type(param("size")) == "string" && param("size") matches "^[1-9][0-9]{0,3}x[1-9][0-9]{0,3}$"`
			const long = `max(float(split(param("size"), "x")[0]), float(split(param("size"), "x")[1]))`
			switch k {
			case "1K":
				return fmt.Sprintf(`(%s || %s == "auto" || %s == "" || %s == nil || (%s && %s <= 1024))`, literal, size, size, size, valid, long)
			case "2K", "4K":
				min, max := 1024, 2048
				if k == "4K" {
					min, max = 2048, 4096
				}
				if s.resolution {
					literal += fmt.Sprintf(" || %s == %q", size, tier)
				}
				return fmt.Sprintf("(%s || (%s && %s > %d && %s <= %d))", literal, valid, long, min, long, max)
			}
		}
		if s.resolution {
			return fmt.Sprintf(`(param("resolution") == %q || param("resolution") == %q)`, k, tier)
		}
		if s.squareAlias && k == "1K" {
			return "(" + literal + ` || param("size") == "1024x1024")`
		}
		return literal
	}
	leaf := func(tier, quality string) string {
		key := tier + "_" + quality
		return fmt.Sprintf("tier(%q, fixed(%s))", key, strconv.FormatFloat(prices[key], 'g', -1, 64))
	}
	qualityChain := func(tier string, guard string) string {
		chain := leaf(tier, s.defaultQuality)
		for i := len(s.qualities) - 1; i >= 0; i-- {
			q := s.qualities[i]
			condition := fmt.Sprintf(`param("quality") == %q`, q)
			if guard != "" {
				condition = guard + " && " + condition
			}
			chain = condition + " ? " + leaf(tier, q) + " : " + chain
		}
		return "(" + chain + ")"
	}
	// Leave a single default leaf even when unmatched dimensions ignore quality.
	var tree string
	if pixels || s.resolution {
		guard := ""
		if !s.resolution {
			guard = sizeCond(tiers[0])
		}
		tree = qualityChain(tiers[0], guard)
		order := []int{1, 2}
		if !pixels && s.resolution {
			order = []int{2, 1}
		}
		for _, i := range order {
			tree = sizeCond(tiers[i]) + " ? " + qualityChain(tiers[i], "") + " : " + tree
		}
	} else {
		tree = leaf(tiers[0], s.defaultQuality)
		for i := 2; i >= 0; i-- {
			tree = sizeCond(tiers[i]) + " ? " + qualityChain(tiers[i], "") + " : " + tree
		}
	}
	return "(" + tree + ") * image_count"
}

// Compare the complete parsed tree with supported legacy and generated forms.
// An extra multiplier, a different default, or a conflicting duplicate leaf
// must remain under the raw editor's control.
func ReadImageSizePrices(expression string) *ImageSizePriceTable {
	return readImageSizePrices(expression, false)
}

// ReadLegacyImageSizePrices prepares a one-time migration draft for the
// explicitly supported old price trees.
func ReadLegacyImageSizePrices(expression string) *ImageSizePriceTable {
	return readImageSizePrices(expression, true)
}

func readImageSizePrices(expression string, legacy bool) *ImageSizePriceTable {
	if !strings.Contains(expression, "image_count") || !strings.Contains(expression, "fixed") {
		return nil
	}
	_, body := ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return nil
	}
	prices := map[string]float64{}
	ast.Find(tree.Node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallNode)
		if !ok || !isFrozenPricingCall(call, "tier", 2) {
			return false
		}
		name, ok := call.Arguments[0].(*ast.StringNode)
		fixed, okFixed := call.Arguments[1].(*ast.CallNode)
		if !ok || !okFixed || !isFrozenPricingCall(fixed, "fixed", 1) {
			return false
		}
		if price, ok := requestRuleNumber(fixed.Arguments[0]); ok {
			prices[name.Value] = price
		}
		return false
	})
	for _, shape := range imagePriceShapes {
		if len(prices) != 3*(1+len(shape.qualities)) {
			continue
		}
		if !legacy {
			if expression == "v1:"+shape.expression(prices, true) {
				return shape.table(prices)
			}
			continue
		}
		if shape.id == "size_only" {
			continue
		}
		expected, err := parser.Parse(shape.expression(prices, false))
		if err == nil && sameImagePriceTree(tree.Node, expected.Node) {
			return shape.table(prices)
		}
	}
	return nil
}

func sameImagePriceTree(left, right ast.Node) bool {
	switch l := left.(type) {
	case *ast.BinaryNode:
		r, ok := right.(*ast.BinaryNode)
		return ok && l.Operator == r.Operator && sameImagePriceTree(l.Left, r.Left) && sameImagePriceTree(l.Right, r.Right)
	case *ast.ConditionalNode:
		r, ok := right.(*ast.ConditionalNode)
		return ok && l.Ternary == r.Ternary && sameImagePriceTree(l.Cond, r.Cond) && sameImagePriceTree(l.Exp1, r.Exp1) && sameImagePriceTree(l.Exp2, r.Exp2)
	case *ast.CallNode:
		r, ok := right.(*ast.CallNode)
		if !ok || len(l.Arguments) != len(r.Arguments) || !sameImagePriceTree(l.Callee, r.Callee) {
			return false
		}
		for i := range l.Arguments {
			if !sameImagePriceTree(l.Arguments[i], r.Arguments[i]) {
				return false
			}
		}
		return true
	case *ast.IdentifierNode:
		r, ok := right.(*ast.IdentifierNode)
		return ok && l.Value == r.Value
	case *ast.StringNode:
		r, ok := right.(*ast.StringNode)
		return ok && l.Value == r.Value
	default:
		a, ok := requestRuleNumber(left)
		b, valid := requestRuleNumber(right)
		return ok && valid && a == b
	}
}

func (s imagePriceShape) table(prices map[string]float64) *ImageSizePriceTable {
	display := []string{"low", "medium", "high"}
	if len(s.qualities) == 0 {
		display = []string{"default"}
	} else if len(s.qualities) > 2 {
		display = append(display, "xhigh", "max")
	}
	return &ImageSizePriceTable{Profile: s.id, Prices: prices, Qualities: display, DefaultQuality: s.defaultQuality, SizeTiers: s.tiers(), UnmatchedUsesQuality: s.resolution}
}

func ImageSizePriceProfiles() []*ImageSizePriceTable {
	tables := make([]*ImageSizePriceTable, 0, len(imagePriceShapes))
	for _, shape := range imagePriceShapes {
		tables = append(tables, shape.table(nil))
	}
	return tables
}

func GenerateImageSizePrices(profile string, prices map[string]float64) (string, error) {
	for _, shape := range imagePriceShapes {
		if shape.id != profile {
			continue
		}
		table := shape.table(prices)
		if len(prices) != len(table.SizeTiers)*len(table.Qualities) {
			return "", errors.New("incomplete image price table")
		}
		for _, size := range table.SizeTiers {
			for _, quality := range table.Qualities {
				key := size + "_" + quality
				value, ok := prices[key]
				if !ok || value < 0 || math.IsNaN(value) || math.IsInf(value*1e6, 0) {
					return "", fmt.Errorf("invalid image price for %s", key)
				}
			}
		}
		expression := "v1:" + shape.expression(prices, true)
		_, err := CompileFromCache(expression)
		return expression, err
	}
	return "", errors.New("unsupported image price profile")
}
