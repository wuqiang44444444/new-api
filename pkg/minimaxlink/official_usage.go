package minimaxlink

import (
	"math"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

const (
	OfficialOutputSeconds     = "output_seconds"
	OfficialInputVideoSeconds = "input_video_seconds"
	OfficialInputImageCount   = "input_image_count"

	OfficialMinOutputSeconds = 4
	OfficialMaxOutputSeconds = 15
	OfficialMaxInputSeconds  = 15
	OfficialMaxImages        = 9
)

func OfficialUsageFields() map[string]jsplugin.UsageFieldSchema {
	return map[string]jsplugin.UsageFieldSchema{
		OfficialOutputSeconds:     {Type: "number", Unit: "second"},
		OfficialInputVideoSeconds: {Type: "number", Unit: "second"},
		OfficialInputImageCount:   {Type: "number", Unit: "count"},
		"resolution":              {Enum: []string{"768p", "2k"}},
	}
}

// ValidOfficialMeasurement checks the absolute protocol bounds. The funding
// owner also checks each amount against the frozen task's input bounds.
func ValidOfficialMeasurement(field string, amount float64) bool {
	switch field {
	case OfficialOutputSeconds:
		return amount >= OfficialMinOutputSeconds && amount <= OfficialMaxOutputSeconds
	case OfficialInputVideoSeconds:
		return amount >= 0 && amount <= OfficialMaxInputSeconds
	case OfficialInputImageCount:
		return amount >= 0 && amount <= OfficialMaxImages && amount == math.Trunc(amount)
	default:
		return false
	}
}
