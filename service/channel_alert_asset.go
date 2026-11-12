package service

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
)

func assetAdapterWithAlert(channel *model.Channel, userID int, modelName string) (*model.Channel, assetadapter.Adapter, error) {
	if !channelAlerts.enabled.Load() || channel == nil {
		adapter, err := seedanceAssetAdapter(channel, userID, modelName)
		return channel, adapter, err
	}
	snapshot := *channel
	adapter, err := seedanceAssetAdapterWithRevision(&snapshot, userID, modelName, &snapshot.AlertAssetRevision)
	return &snapshot, adapter, err
}

func observeAssetChannelAlert(ctx context.Context, operation, modelName string, channel *model.Channel, err error) {
	if !channelAlerts.enabled.Load() || channel == nil || channel.AlertAssetRevision == "" || ctx.Err() != nil {
		return
	}
	fields, classified := assetadapter.UpstreamDiagnosticFields(err)
	if err != nil && !classified {
		return
	}
	transport := fields.Class == assetadapter.AssetClassTimeout || fields.Class == assetadapter.AssetClassConnect || fields.Class == assetadapter.AssetClassReset
	kind := classifyChannelAlertHTTP(fields.StatusCode, transport)
	if err == nil || kind != channelAlertSuccess {
		id, _ := ctx.Value(common.RequestIdKey).(string)
		if id == "" {
			id = common.NewRequestId()
		}
		recordChannelAlert(channelAlertScope{Channel: channel.Id, Revision: channel.AlertAssetRevision, Model: modelName, Operation: operation, Slot: -1},
			id, kind, fields.StatusCode)
	}
}
