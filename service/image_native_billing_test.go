package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeImageExpressionPreservesOriginalProbeAndUsage(t *testing.T) {
	restoreRuntime(t)
	store := &sessionTestImageStore{}
	taskArtifactStoreRuntime.swap(store, "native-billing")
	task := newWorkerImageTask(9934, 100)
	snapshot := tieredTestSnapshot(`tier("native", (p + c) * (param("model") == "customer-image" && param("output_compression") == 0 && param("extra.factor") == 2 && header("x-factor") == "double" ? 4 : 1))`, 100)
	task.PrivateData.BillingContext = &model.TaskBillingContext{TieredSnapshot: snapshot}
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.PrivateData.ImageTask.NativeRequest = &model.TaskNativeImageRequest{}
	info := &relaycommon.RelayInfo{OriginModelName: "customer-image", TieredBillingSnapshot: snapshot, BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"model":"customer-image","output_compression":0,"extra":{"factor":2},"image":"private-input-reference"}`), Headers: map[string]string{"X-Factor": "double", "Authorization": "billing-fixture-secret"}}}
	require.NoError(t, FreezeImageTaskBilling(t.Context(), task, info, &dto.ImageRequest{Model: "mapped"}))
	encoded, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-input-reference")
	assert.NotContains(t, string(encoded), "billing-fixture-secret")
	for _, object := range store.objects {
		assert.NotContains(t, string(object), "billing-fixture-secret")
		assert.NotContains(t, string(object), "private-input-reference")
	}
	restored, err := common.DeepCopy(task)
	require.NoError(t, err)
	info.BillingRequestInput.Body = []byte(`{}`)
	info.BillingRequestInput.Headers["X-Factor"] = "changed"
	quota, clamp, err := imageTaskTargetQuota(t.Context(), restored, &dto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 300, quota)
	store.objects = map[string][]byte{}
	_, _, err = imageTaskTargetQuota(t.Context(), restored, &dto.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	require.Error(t, err, "missing billing evidence must keep settlement pending")
}
