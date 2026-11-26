package claude

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ClaudeResponsesStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseID := helper.GetResponseID(c)
	created := common.GetTimestamp()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:                 responseID,
		Model:              info.UpstreamModelName,
		Created:            created,
		EmitSequenceNumber: true,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	hostedBridge := relayconvert.NewClaudeHostedStreamBridge()

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   responseID,
		Created:      created,
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	var streamErr *types.NewAPIError
	var delivery helper.ResponsesDelivery
	defer func() { delivery.Finish(info.StreamStatus) }()
	var providerStopped bool
	// A protocol error ends the stream with the usage already reported upstream.
	streamFailed := false
	stopStream := func(sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		sr.Stop(nil)
	}

	sendResponsesEvent := func(eventType string, payload dto.ResponsesStreamResponse) bool {
		delivered, err := delivery.Send(c, info, eventType, payload)
		streamErr = err
		return delivered
	}
	sendResult := func(result relayconvert.ResponseResult) bool {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			streamErr = types.NewOpenAIError(
				fmt.Errorf("expected OpenAI Responses stream event, got %T", result.Value),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			)
			return false
		}
		return sendResponsesEvent(event.Type, event.Payload)
	}
	failResponsesStream := func(err error) bool {
		failureResults, handled := state.FailResponsesStream("server_error", err.Error(), "")
		if !handled {
			return false
		}
		streamFailed = true
		for _, result := range failureResults {
			if !sendResult(result) {
				return true
			}
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var claudeResponse dto.ClaudeResponse
		if err := common.UnmarshalJsonStr(data, &claudeResponse); err != nil {
			logger.LogError(c, common.LogText("failed to unmarshal Claude stream event: %s", err.Error()))
			if failResponsesStream(err) {
				// A nil streamErr here is intentional: the protocol-level failure
				// event was delivered, so only the scanner needs to stop.
				stopStream(sr)
				return
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
			if failResponsesStream(fmt.Errorf("%s", claudeError.Message)) {
				stopStream(sr)
				return
			}
			streamErr = types.WithClaudeError(*claudeError, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}

		if claudeResponse.StopReason != "" {
			maybeMarkClaudeRefusal(c, info, claudeResponse.StopReason)
		}
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			maybeMarkClaudeRefusal(c, info, *claudeResponse.Delta.StopReason)
		}
		if claudeResponse.Type == "message_stop" {
			providerStopped = true
		}
		if claudeResponse.Type == "message_start" && claudeResponse.Message != nil {
			info.ObserveResponseModel(claudeResponse.Message.Model)
			info.UpstreamModelName = claudeResponse.Message.Model
		}
		FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo)
		countClaudeStreamBillableTools(info, &claudeResponse)
		hostedEvents, consumed, err := hostedBridge.Convert(&claudeResponse, state)
		if err != nil {
			if failResponsesStream(err) {
				stopStream(sr)
				return
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, event := range hostedEvents {
			if !sendResponsesEvent(event.Type, event.Payload) {
				stopStream(sr)
				return
			}
		}
		if consumed {
			return
		}

		results, err := service.ConvertStreamResponseChunk(c, info, state, &claudeResponse)
		if err != nil {
			if failResponsesStream(err) {
				stopStream(sr)
				return
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendResult(result) {
				stopStream(sr)
				return
			}
		}
	})
	info.StreamStatus.RequireTerminal()
	if streamErr != nil {
		return nil, streamErr
	}
	if streamFailed {
		return claudeInfo.Usage, nil
	}

	HandleStreamFinalResponse(c, info, claudeInfo)
	if delivery.Disconnected(info.StreamStatus) {
		return claudeInfo.Usage, nil
	}
	openAIUsage := claudeInfo.OpenAIUsage()
	state.SetUsage(&openAIUsage)
	if !providerStopped || !info.StreamStatus.IsNormalEnd() {
		err := errors.New("Claude stream ended before completion")
		if failResponsesStream(err) {
			return claudeInfo.Usage, streamErr
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	finalResults, err := service.FinalizeStreamResponse(c, info, state)
	if err != nil {
		if failResponsesStream(err) {
			return claudeInfo.Usage, streamErr
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if !sendResult(result) {
			return claudeInfo.Usage, streamErr
		}
	}
	return claudeInfo.Usage, nil
}
