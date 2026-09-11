package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	pluginadaptor "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskBillingFreezesSelectedAdapterUsageUnits(t *testing.T) {
	plugin := &pluginruntime.LoadedPlugin{Meta: pluginruntime.Meta{UsageSchema: map[string]pluginruntime.UsageFieldSchema{
		"meter": {Type: "number", Unit: "token"}, "mode": {Enum: []string{"pro", "standard"}}, "audio": {Type: "boolean"},
	}}}
	snapshot := &billingexpr.BillingSnapshot{TaskUsageBilling: true, ExprString: `tier("base", u("meter") * 7 / 1000000)`}
	freezeTaskBillingUsageUnits(snapshot, pluginadaptor.New(plugin))
	plugin.Meta.UsageSchema["meter"] = pluginruntime.UsageFieldSchema{Type: "number", Unit: "credit"}
	encoded, err := common.Marshal(snapshot)
	require.NoError(t, err)
	var restored billingexpr.BillingSnapshot
	require.NoError(t, common.Unmarshal(encoded, &restored))
	assert.Equal(t, map[string]string{"meter": "token", "mode": "enum", "audio": "boolean"}, restored.UsageUnits)
}
