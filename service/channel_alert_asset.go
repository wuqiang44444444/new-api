package service

import (
	"context"
	"strings"

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
	if err != nil && upstreamBalanceRejected(fields.StatusCode, fields.ProviderCode, "") {
		kind = channelAlertBalance
	}
	if err == nil || kind != channelAlertSuccess {
		id, _ := ctx.Value(common.RequestIdKey).(string)
		if id == "" {
			id = common.NewRequestId()
		}
		recordChannelAlert(channelAlertScope{Channel: channel.Id, Revision: channel.AlertAssetRevision, Model: modelName, Operation: operation, Slot: -1},
			id, kind, fields.StatusCode)
	}
}

// Keep the adapter's raw error available to the balance classifier before the
// plugin maps it into the public asset contract.
func assetBalanceObserver(channelID int, revision, modelName string) func(context.Context, string, int, []byte) {
	return func(ctx context.Context, operation string, status int, body []byte) {
		if ctx.Err() != nil || !upstreamBalanceResponse(status, channelBalanceProfile{}, body, true) {
			return
		}
		action, resource, _ := strings.Cut(operation, "_")
		scope := channelAlertScope{Channel: channelID, Revision: revision, Model: modelName, Operation: "asset_" + action, Slot: -1}
		switch resource {
		case "group", "groups":
			scope.Operation = "asset_group_" + action
		case "verification", "verification_session":
			scope.Operation = "asset_verification_" + action
		}
		if action == "list" {
			scope.Operation = "asset_probe"
		}
		recordChannelAlert(scope, common.NewRequestId(), channelAlertBalance, status)
	}
}
