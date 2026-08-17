package billing_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func genericBillingSmokeRequests() []billingexpr.RequestInput {
	return []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{"anthropic-beta": "fast-mode-2026-02-01"},
			Body:    []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}
}

// taskBillingSmokeRequests mirrors the fields common to every Seedance task
// billing probe. Protocol-only fields are supplied by the selected channel and
// added to every variant, so validation never advertises them to other protocols.
func taskBillingSmokeRequests(extraFields map[string]any) ([]billingexpr.RequestInput, error) {
	variants := []map[string]any{
		{"resolution": "480p", "has_video_input": false, "generate_audio": false, "input_mode": "text", "control_mode": "none"},
		{"resolution": "720p", "has_video_input": true, "generate_audio": false, "input_mode": "video", "control_mode": "reference"},
		{"resolution": "1080p", "has_video_input": false, "generate_audio": true, "input_mode": "text", "control_mode": "none"},
		{"resolution": "1080p", "has_video_input": true, "generate_audio": true, "input_mode": "video", "control_mode": "reference"},
		{"resolution": "4k", "has_video_input": false, "generate_audio": false, "input_mode": "text", "control_mode": "none"},
		{"resolution": "4k", "has_video_input": true, "generate_audio": false, "input_mode": "video", "control_mode": "reference"},
		{"resolution": "720p", "has_video_input": false, "generate_audio": false, "input_mode": "text", "control_mode": "none"},
	}
	requests := make([]billingexpr.RequestInput, 0, len(variants))
	for index, variant := range variants {
		taskProbe := make(map[string]any, len(extraFields)+len(variant)+1)
		for field, value := range extraFields {
			taskProbe[field] = value
		}
		for field, value := range variant {
			taskProbe[field] = value
		}
		taskProbe["duration_seconds"] = 5

		body := map[string]any{"_task": taskProbe}
		request := billingexpr.RequestInput{}
		if index == len(variants)-1 {
			request.Headers = map[string]string{"anthropic-beta": "fast-mode-2026-02-01"}
			body["service_tier"] = "fast"
			body["stream_options"] = map[string]any{"include_usage": true}
			body["messages"] = make([]int, 21)
		}
		encoded, err := common.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal billing smoke request: %w", err)
		}
		request.Body = encoded
		requests = append(requests, request)
	}
	return requests, nil
}
