// Package funcloud implements the Seedance FunCloud protocol.
package funcloud

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

type TaskResponseContext struct {
	ProviderModel string
	Resolution    string
	HasVideoInput bool
	MaxTokens     int
}

func TaskResponse(body []byte, expectedTaskID string, responseContext TaskResponseContext) ([]byte, error) {
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID           string   `json:"taskId"`
			Status           string   `json:"status"`
			Result           []string `json:"result"`
			ErrorCode        string   `json:"errorCode"`
			ErrorMsg         string   `json:"errorMsg"`
			CompletionTokens *int     `json:"completionTokens"`
			PointConsume     string   `json:"pointConsume"`
			Output           struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Content struct {
					VideoURL string `json:"video_url"`
				} `json:"content"`
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
				PointConsume string `json:"pointConsume"`
			} `json:"output"`
		} `json:"data"`
	}
	violation := func(reason string) ([]byte, error) {
		return nil, &relaycommon.UpstreamContractViolation{Reason: reason}
	}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return violation("invalid FunCloud task response")
	}
	if envelope.Code != 0 {
		return violation("FunCloud task response contains an application error")
	}
	taskID := strings.TrimSpace(envelope.Data.TaskID)
	if !validTaskID(taskID) || taskID != expectedTaskID {
		return violation("FunCloud task id mismatch")
	}
	if outputID := strings.TrimSpace(envelope.Data.Output.ID); outputID != "" && !validTaskID(outputID) {
		return violation("FunCloud output task id is invalid")
	}

	status, ok := normalizedStatus(envelope.Data.Status)
	if !ok {
		return violation("unsupported FunCloud task status")
	}
	if outputStatus := strings.TrimSpace(envelope.Data.Output.Status); outputStatus != "" {
		normalizedOutput, outputOK := normalizedStatus(outputStatus)
		if !outputOK || normalizedOutput != status {
			return violation("conflicting FunCloud task statuses")
		}
	}

	videoURL := strings.TrimSpace(envelope.Data.Output.Content.VideoURL)
	resultURL := ""
	if len(envelope.Data.Result) == 1 {
		resultURL = strings.TrimSpace(envelope.Data.Result[0])
	} else if len(envelope.Data.Result) > 1 {
		return violation("FunCloud task response contains multiple result URLs")
	}
	if videoURL != "" && resultURL != "" && videoURL != resultURL {
		return violation("conflicting FunCloud result URLs")
	}
	if videoURL == "" {
		videoURL = resultURL
	}
	if status == "succeeded" {
		if len(envelope.Data.Result) == 0 && videoURL == "" {
			return violation("FunCloud task response has no successful result URL")
		}
		var err error
		videoURL, err = relaycommon.ValidateHTTPSVideoResultURL(videoURL)
		if err != nil {
			return violation("invalid FunCloud result URL")
		}
	}

	standardErrorCode := strings.TrimSpace(envelope.Data.Output.Error.Code)
	fastErrorCode := strings.TrimSpace(envelope.Data.ErrorCode)
	if standardErrorCode != "" && fastErrorCode != "" && standardErrorCode != fastErrorCode {
		return violation("conflicting FunCloud task error codes")
	}
	errorMessage := strings.TrimSpace(envelope.Data.Output.Error.Message)
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(envelope.Data.ErrorMsg)
	} else if fastMessage := strings.TrimSpace(envelope.Data.ErrorMsg); fastMessage != "" && fastMessage != errorMessage {
		return violation("conflicting FunCloud task error messages")
	}
	errorMessage = sanitizeProviderText(errorMessage)
	errorCode := standardErrorCode
	if errorCode == "" {
		errorCode = fastErrorCode
	}
	errorCode = sanitizeProviderText(errorCode)
	completionTokens := 0
	var billingEvidence *relaycommon.ProviderBillingEvidence
	if status == "succeeded" {
		if envelope.Data.CompletionTokens == nil || *envelope.Data.CompletionTokens <= 0 ||
			responseContext.MaxTokens <= 0 || *envelope.Data.CompletionTokens > responseContext.MaxTokens {
			return violation("FunCloud completionTokens is not trustworthy")
		}
		completionTokens = *envelope.Data.CompletionTokens
		pointConsume := strings.TrimSpace(envelope.Data.PointConsume)
		outputPointConsume := strings.TrimSpace(envelope.Data.Output.PointConsume)
		if pointConsume == "" {
			pointConsume = outputPointConsume
		} else if outputPointConsume != "" && outputPointConsume != pointConsume {
			primary, primaryErr := decimal.NewFromString(pointConsume)
			output, outputErr := decimal.NewFromString(outputPointConsume)
			if primaryErr != nil || outputErr != nil || !primary.Equal(output) {
				return violation("conflicting FunCloud point consumption values")
			}
		}
		if pointConsume != "" {
			points, err := decimal.NewFromString(pointConsume)
			if err != nil || !points.IsPositive() {
				return violation("FunCloud point consumption is not trustworthy")
			}
		}
		billingEvidence = &relaycommon.ProviderBillingEvidence{
			Provider:        "funcloud",
			TokenSource:     "completionTokens",
			ReportedTokens:  completionTokens,
			RawConsumption:  pointConsume,
			ConsumptionUnit: "pointConsume",
			ProviderModel:   strings.TrimSpace(responseContext.ProviderModel),
			Resolution:      strings.ToLower(strings.TrimSpace(responseContext.Resolution)),
			HasVideoInput:   responseContext.HasVideoInput,
		}
		if pointConsume == "" {
			billingEvidence.ConsumptionUnit = ""
		}
	}
	return common.Marshal(struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url,omitempty"`
		} `json:"content"`
		Error struct {
			Code    string `json:"code,omitempty"`
			Message string `json:"message,omitempty"`
		} `json:"error"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens,omitempty"`
			TotalTokens      int `json:"total_tokens,omitempty"`
		} `json:"usage"`
		ProviderBillingEvidence *relaycommon.ProviderBillingEvidence `json:"_provider_billing_evidence,omitempty"`
	}{
		ID:     taskID,
		Status: status,
		Content: struct {
			VideoURL string `json:"video_url,omitempty"`
		}{VideoURL: videoURL},
		Error: struct {
			Code    string `json:"code,omitempty"`
			Message string `json:"message,omitempty"`
		}{Code: errorCode, Message: errorMessage},
		Usage: struct {
			CompletionTokens int `json:"completion_tokens,omitempty"`
			TotalTokens      int `json:"total_tokens,omitempty"`
		}{CompletionTokens: completionTokens, TotalTokens: completionTokens},
		ProviderBillingEvidence: billingEvidence,
	})
}

func normalizedStatus(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "processing", "running", "submitted":
		return "running", true
	case "success", "completed", "succeeded":
		return "succeeded", true
	case "failed":
		return "failed", true
	default:
		return "", false
	}
}

func sanitizeProviderText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	for _, unsafe := range []string{"http://", "https://", "authorization", "bearer ", "cookie", "token="} {
		if strings.Contains(lower, unsafe) {
			return "upstream task failed"
		}
	}
	runes := []rune(value)
	if len(runes) > 512 {
		value = string(runes[:512])
	}
	return value
}
