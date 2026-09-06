package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pkgbilling "github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTaskVideoDetailsMissingRecords(t *testing.T) {
	// 完全没有快照与计费事实的历史任务：不补造任何参数。
	details := buildTaskVideoDetails(&model.Task{})
	require.Nil(t, details)
	details = buildTaskVideoDetails(nil)
	require.Nil(t, details)
}

func TestBuildTaskVideoDetailsExplicitFalseVsMissing(t *testing.T) {
	explicitFalse := false
	explicitTrue := true

	details := buildTaskVideoDetails(&model.Task{
		PrivateData: model.TaskPrivateData{
			ClientRequest: &model.TaskClientRequestSnapshot{
				Seconds:       "5",
				Resolution:    "1080p",
				Ratio:         "16:9",
				GenerateAudio: &explicitFalse,
			},
		},
	})
	require.NotNil(t, details)
	require.NotNil(t, details.Request)
	require.NotNil(t, details.Request.Seconds)
	assert.Equal(t, "5", details.Request.Seconds.Value)
	require.NotNil(t, details.Request.Resolution)
	assert.Equal(t, "1080p", details.Request.Resolution.Value)
	require.NotNil(t, details.Request.Ratio)
	assert.Equal(t, "16:9", details.Request.Ratio.Value)
	// 显式 false 必须保留为 false，而不是显示成缺失。
	require.NotNil(t, details.Request.GenerateAudio)
	assert.False(t, details.Request.GenerateAudio.Value)

	// 缺失的 generate_audio 与显式 false 严格区分。
	details = buildTaskVideoDetails(&model.Task{
		PrivateData: model.TaskPrivateData{
			ClientRequest: &model.TaskClientRequestSnapshot{
				Seconds:       "5",
				GenerateAudio: &explicitTrue,
			},
		},
	})
	require.NotNil(t, details.Request.GenerateAudio)
	assert.True(t, details.Request.GenerateAudio.Value)
	assert.Nil(t, details.Request.Resolution)
	assert.Nil(t, details.Request.Ratio)

	details = buildTaskVideoDetails(&model.Task{
		PrivateData: model.TaskPrivateData{
			ClientRequest: &model.TaskClientRequestSnapshot{ServiceTier: "default"},
		},
	})
	require.NotNil(t, details.Request)
	require.NotNil(t, details.Request.ServiceTier)
	assert.Equal(t, "default", details.Request.ServiceTier.Value)
	assert.Nil(t, details.Request.GenerateAudio)
}

func TestBuildTaskVideoDetailsBillingProbe(t *testing.T) {
	// 计费探针是“计费采用参数”：包含默认值与换算结果，独立成组投影。
	details := buildTaskVideoDetails(&model.Task{
		Quota: 1500,
		PrivateData: model.TaskPrivateData{
			AsyncBilling: &model.TaskAsyncBillingContext{
				State: model.TaskBillingStateSettled,
				BillingProbe: &pkgbilling.RequestInput{
					Body: []byte(`{"_task":{"duration_seconds":5,"resolution":"720p","generate_audio":true}}`),
				},
			},
			BillingContext: &model.TaskBillingContext{
				OtherRatios: map[string]float64{"seconds": 5, "resolution": 1},
			},
		},
	})
	require.NotNil(t, details)
	require.NotNil(t, details.Billing)
	require.NotNil(t, details.Billing.DurationSeconds)
	assert.Equal(t, "5", details.Billing.DurationSeconds.Value)
	require.NotNil(t, details.Billing.Resolution)
	assert.Equal(t, "720p", details.Billing.Resolution.Value)
	require.NotNil(t, details.Billing.GenerateAudio)
	assert.True(t, details.Billing.GenerateAudio.Value)

	require.NotNil(t, details.Settlement)
	assert.Equal(t, 1500, details.Settlement.Quota)
	assert.Equal(t, string(model.TaskBillingStateSettled), details.Settlement.BillingState)
	assert.False(t, details.Settlement.ActualUsageReported)
	require.Len(t, details.Settlement.OtherRatios, 2)

	// 探针缺失时不补造计费参数。
	details = buildTaskVideoDetails(&model.Task{
		PrivateData: model.TaskPrivateData{
			AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	})
	require.NotNil(t, details)
	assert.Nil(t, details.Billing)
}

func TestBuildTaskVideoDetailsActualUsageSettlement(t *testing.T) {
	details := buildTaskVideoDetails(&model.Task{
		PrivateData: model.TaskPrivateData{
			AsyncBilling: &model.TaskAsyncBillingContext{
				State:               model.TaskBillingStateSettled,
				ActualUsageReported: true,
				ActualUsageEvidence: map[string]int{"duration_seconds": 8},
			},
		},
	})
	require.NotNil(t, details.Settlement)
	assert.True(t, details.Settlement.ActualUsageReported)
	assert.Equal(t, map[string]int{"duration_seconds": 8}, details.Settlement.ActualUsage)
}

func TestTasksToDtoRoleProjection(t *testing.T) {
	audioFalse := false
	tasks := []*model.Task{{
		TaskID: "task_demo",
		Quota:  120,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:           "up-1",
			UpstreamRequestID:        "req-up-1",
			VideoUpstreamProtocol:    "moxing_media_v1",
			SouthboundAdapterVersion: "seedance/moxing@3",
			ClientRequest: &model.TaskClientRequestSnapshot{
				Seconds:       "4",
				GenerateAudio: &audioFalse,
			},
		},
	}}

	userItems := tasksToDto(tasks, false, common.RoleCommonUser)
	require.Len(t, userItems, 1)
	assert.Nil(t, userItems[0].AdminInfo)
	assert.Nil(t, userItems[0].RootInfo)
	require.NotNil(t, userItems[0].VideoDetails)
	require.NotNil(t, userItems[0].VideoDetails.Request.GenerateAudio)
	assert.False(t, userItems[0].VideoDetails.Request.GenerateAudio.Value)

	adminItems := tasksToDto(tasks, false, common.RoleAdminUser)
	require.Len(t, adminItems, 1)
	assert.Nil(t, adminItems[0].RootInfo)

	rootItems := tasksToDto(tasks, false, common.RoleRootUser)
	require.Len(t, rootItems, 1)
	require.NotNil(t, rootItems[0].RootInfo)
	assert.Equal(t, "up-1", rootItems[0].RootInfo.UpstreamTaskID)
	assert.Equal(t, "req-up-1", rootItems[0].RootInfo.UpstreamRequestID)
	assert.Equal(t, "moxing_media_v1", rootItems[0].RootInfo.VideoUpstreamProtocol)
	assert.Equal(t, "seedance/moxing@3", rootItems[0].RootInfo.SouthboundAdapterVersion)
}

func TestTasksToDtoWithoutLinkFactsOmitsRootInfo(t *testing.T) {
	// 普通插件任务没有 Link 冻结诊断字段时不应伪造空 RootInfo；
	// quota 是既有结算事实，仍作为 Settlement 投影。
	tasks := []*model.Task{{TaskID: "task_plugin", Quota: 10}}
	rootItems := tasksToDto(tasks, false, common.RoleRootUser)
	require.Len(t, rootItems, 1)
	assert.Nil(t, rootItems[0].RootInfo)
	require.NotNil(t, rootItems[0].VideoDetails)
	require.NotNil(t, rootItems[0].VideoDetails.Settlement)
	assert.Equal(t, 10, rootItems[0].VideoDetails.Settlement.Quota)
	assert.Nil(t, rootItems[0].VideoDetails.Request)
	assert.Nil(t, rootItems[0].VideoDetails.Billing)
}
