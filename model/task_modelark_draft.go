package model

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func mergeDraftUsageObservation(accepted, incoming json.RawMessage, newlyMeasured bool) (json.RawMessage, error) {
	if !newlyMeasured {
		return accepted, nil
	}
	var current struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := common.Unmarshal(incoming, &current); err != nil {
		return nil, err
	}
	if len(current.Usage) == 0 {
		return accepted, nil
	}
	var result map[string]json.RawMessage
	if err := common.Unmarshal(accepted, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("missing accepted video draft observation")
	}
	result["usage"] = current.Usage
	return common.Marshal(result)
}

type ModelArkDraftObservation uint8

const (
	ModelArkDraftUnconfirmed ModelArkDraftObservation = iota
	ModelArkConfirmedDraft
	ModelArkConfirmedOrdinary
)

// ModelArkDraftMarker interprets a provider observation, not a request echo.
func ModelArkDraftMarker(data []byte) ModelArkDraftObservation {
	var observation struct {
		Draft *bool `json:"draft"`
	}
	if common.Unmarshal(data, &observation) != nil || observation.Draft == nil {
		return ModelArkDraftUnconfirmed
	}
	if *observation.Draft {
		return ModelArkConfirmedDraft
	}
	return ModelArkConfirmedOrdinary
}

func (t *Task) IsVideoDraftOperation() bool {
	return t.PrivateData.VideoDraftRequested || t.PrivateData.VideoDraftSourceID != ""
}

func freezeTaskVideoDraft(data *TaskPrivateData, info *relaycommon.RelayInfo) {
	if !info.TaskRelayInfo.IsVideoDraftOperation() {
		return
	}
	data.VideoDraftRequested = info.VideoDraftRequested
	data.VideoDraftSourceID = info.VideoDraftSourceID
	data.VideoDraftCreatePath = info.ChannelOtherSettings.VideoUpstreamCreatePath
}

func (t *Task) IsSeedanceDraftSource() bool {
	return string(t.Platform) == strconv.Itoa(constant.ChannelTypeSeedanceLink) && t.PrivateData.VideoDraftRequested
}
