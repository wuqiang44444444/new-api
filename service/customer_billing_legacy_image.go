package service

import "math"

func appendLegacyImageOutputLines(lines []customerBillingLine, output customerBillingLine, usage map[string]any) []customerBillingLine {
	text, _ := customerBillingNumber(usage, "text_output")
	image, _ := customerBillingNumber(usage, "image_output")
	if text+image != output.Quantity || math.Trunc(text) != text || math.Trunc(image) != image {
		return append(lines, output)
	}
	// Legacy settlement applies the same output rate to both modalities.
	price := output.UnitPrice
	if text > 0 {
		lines = append(lines, customerBillingLine{"Text Output", text, "token", price, text * price / 1000000})
	}
	if image > 0 {
		lines = append(lines, customerBillingLine{"Image Out", image, "token", price, image * price / 1000000})
	}
	return lines
}
