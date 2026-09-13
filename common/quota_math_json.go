package common

import (
	"encoding/json"
	"fmt"
	"math"
)

// JSON has no non-finite numbers. Preserve the diagnostic value as a named
// string so a rejected calculation cannot prevent task or audit persistence.
func quotaClampJSONValue(value float64) any {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "+Inf"
	case math.IsInf(value, -1):
		return "-Inf"
	default:
		return value
	}
}

func (c QuotaClamp) MarshalJSON() ([]byte, error) {
	return Marshal(c.AuditMap())
}

func (c *QuotaClamp) UnmarshalJSON(data []byte) error {
	var fields struct {
		Op       string          `json:"op"`
		Kind     QuotaClampKind  `json:"kind"`
		Original json.RawMessage `json:"original"`
		Clamped  int             `json:"clamped"`
	}
	if err := Unmarshal(data, &fields); err != nil {
		return err
	}
	var original float64
	if err := Unmarshal(fields.Original, &original); err != nil {
		var named string
		if err := Unmarshal(fields.Original, &named); err != nil {
			return fmt.Errorf("invalid quota clamp original value")
		}
		switch named {
		case "NaN":
			original = math.NaN()
		case "+Inf":
			original = math.Inf(1)
		case "-Inf":
			original = math.Inf(-1)
		default:
			return fmt.Errorf("invalid quota clamp original value")
		}
	}
	*c = QuotaClamp{Op: fields.Op, Kind: fields.Kind, Original: original, Clamped: fields.Clamped}
	return nil
}
