package service

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// Browser estimators already hold billable token dimensions. This explicit
// simulation-only input avoids normalizing cache counts twice; provider usage
// continues through BuildTieredTokenParams in the existing preview path.
func normalizedPreviewTokens(sample *BillingExprPreviewSample) (billingexpr.TokenParams, error) {
	var params billingexpr.TokenParams
	if sample.NormalizedUsage == nil {
		return params, errors.New("Normalized token counts are required.")
	}
	if sample.PromptTokens != 0 || sample.CompletionTokens != 0 || sample.CacheReadTokens != 0 ||
		sample.CacheCreationTokens != 0 || sample.CacheCreation1hTokens != 0 || sample.ImageTokens != 0 ||
		sample.ImageOutputTokens != 0 || sample.AudioInputTokens != 0 || sample.AudioOutputTokens != 0 {
		return params, errors.New("Do not combine normalized token counts with provider usage counts.")
	}
	fields := map[string]*float64{
		"p": &params.P, "c": &params.C, "len": &params.Len,
		"cr": &params.CR, "cc": &params.CC, "cc1h": &params.CC1h,
		"img": &params.Img, "img_cr": &params.ImgCR, "img_o": &params.ImgO,
		"ai": &params.AI, "ao": &params.AO,
	}
	for name, value := range sample.NormalizedUsage {
		field, exists := fields[name]
		if !exists {
			return params, errors.New("Unknown normalized token variable.")
		}
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > math.MaxInt32 {
			return params, errors.New("Token counts must be finite and between 0 and 2147483647.")
		}
		*field = *value
	}
	return params, nil
}
