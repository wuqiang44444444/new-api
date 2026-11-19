package billingexpr

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageSizePricesRoundTripAndFrozenReplay(t *testing.T) {
	for _, profile := range ImageSizePriceProfiles() {
		t.Run(profile.Profile, func(t *testing.T) {
			prices := make(map[string]float64)
			for row, tier := range profile.SizeTiers {
				for column, quality := range profile.Qualities {
					prices[tier+"_"+quality] = float64(row*10+column+1) / 100
				}
			}
			expression, err := GenerateImageSizePrices(profile.Profile, prices)
			require.NoError(t, err)
			table := ReadImageSizePrices(expression)
			require.NotNil(t, table)
			assert.Equal(t, prices, table.Prices)
			assert.Nil(t, ReadImageSizePrices(expression+" "))
			assert.Empty(t, DiagnoseImagePrices(expression))

			for _, example := range []struct {
				size, quality string
				row           int
				invalid       bool
			}{
				{"1024x99", "high", 0, false},
				{"99x1025", "low", 1, false},
				{"2048x100", "auto", 1, false},
				{"100x2049", "", 2, false},
				{"4096x1024", "high", 2, false},
				{"4097x1024", "low", 0, true},
				{"3840X2160", "high", 0, true},
				{" 2048x1024", "low", 0, true},
				{"02048x1024", "high", 0, true},
				{"0x2048", "high", 0, true},
				{"1K", "high", 0, false},
				{"2K", "low", 1, false},
				{"4K", "", 2, false},
				{"2k", "high", 1, false},
				{"4k", "low", 2, false},
				{"auto", "high", 0, false},
				{"", "low", 0, false},
				{"", "", 0, false},
			} {
				t.Run(example.size+"/"+example.quality, func(t *testing.T) {
					body := map[string]string{}
					if example.size != "" {
						body["size"] = example.size
					}
					if example.quality != "" {
						body["quality"] = example.quality
					}
					raw, err := common.Marshal(body)
					require.NoError(t, err)
					row, quality := example.row, example.quality
					if !table.UnmatchedUsesQuality && (example.size == "2k" || example.size == "4k") {
						row, quality = 0, table.DefaultQuality
					}
					if len(table.Qualities) == 1 || quality == "" || quality == "auto" || (example.invalid && !table.UnmatchedUsesQuality) {
						quality = table.DefaultQuality
					}
					expectedTier := table.SizeTiers[row] + "_" + quality
					n := 2
					reserved, trace, err := RunExprWithRequest(expression, TokenParams{P: 1000, C: 9000}, RequestInput{Body: raw, ImageCount: &n})
					require.NoError(t, err)
					assert.Equal(t, expectedTier, trace.MatchedTier)
					assert.Equal(t, BillingUnitRequest, trace.BillingUnit)
					assert.InDelta(t, 2*prices[expectedTier]*1e6, reserved, 1e-6)
					n = 1
					frozen, _, err := WithFrozenRequestRules(expression, RequestInput{ImageCount: &n}, trace.RequestRules, trace.MatchedTier, trace.FixedPrice)
					require.NoError(t, err)
					settled, _, err := RunExprWithRequest(expression, TokenParams{}, frozen)
					require.NoError(t, err)
					assert.InDelta(t, reserved/2, settled, 1e-6)
				})
			}
		})
	}
}

func TestImageSizePriceTablePreservesEqualPricedTiers(t *testing.T) {
	table := ImageSizePriceProfiles()[0]
	prices := make(map[string]float64)
	for _, size := range table.SizeTiers {
		for _, quality := range table.Qualities {
			prices[size+"_"+quality] = 0.05
		}
	}
	expr, err := GenerateImageSizePrices(table.Profile, prices)
	require.NoError(t, err)
	reopened := ReadImageSizePrices(expr)
	require.NotNil(t, reopened)
	assert.Equal(t, prices, reopened.Prices)
	prices["4K_high"] = -1
	_, err = GenerateImageSizePrices(table.Profile, prices)
	assert.Error(t, err)
}

func TestImagePriceDiagnosticsPreserveIntentionalUniformPrices(t *testing.T) {
	assert.Empty(t, DiagnoseImagePrices(`tier("image", fixed(0.05)) * image_count`))
	assert.ElementsMatch(t, []ImagePriceWarning{
		{Code: "unavailable_parameter", Field: "resolution"},
		{Code: "conflicting_tier", Field: "image"},
		{Code: "token_input", Field: "p"},
	}, DiagnoseImagePrices(`((param("resolution") == "4K" || p > 1) ? tier("image", fixed(0.07)) : tier("image", fixed(0.05))) * image_count`))
}

func TestImagePriceDiagnosticsUseFrozenPerImageFields(t *testing.T) {
	for _, field := range []string{"size", "quality", "model", "n", "parameters.n", "parameters.prompt_extend", "input_image_count", "background", "parameters.other"} {
		expression := fmt.Sprintf(`(param(%q) == nil ? tier("a", fixed(1)) : tier("b", fixed(2))) * image_count`, field)
		switch field {
		case "input_image_count", "background", "parameters.other":
			assert.Equal(t, []ImagePriceWarning{{Code: "unavailable_parameter", Field: field}}, DiagnoseImagePrices(expression))
		default:
			assert.Empty(t, DiagnoseImagePrices(expression))
		}
	}
	// Typed relay pricing without image_count keeps its protocol-specific request input.
	assert.Empty(t, DiagnoseImagePrices(`tier("a", fixed(1)) * (param("input_image_count") == nil ? 1 : 2)`))
}
