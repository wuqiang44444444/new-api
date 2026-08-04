package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/provider_exposure_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderExposurePolicyPagesAndDisablesOnlyAffectedPublicModel(t *testing.T) {
	truncate(t)
	setting := provider_exposure_setting.GetSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })
	*setting = provider_exposure_setting.PolicySetting{
		Enabled:                       true,
		MonitoredImplementations:      model.LinkImplementationFeicaiSeedanceVideos + "/" + model.LinkImplementationVersionV1,
		WindowSeconds:                 3600,
		WarningCount:                  1,
		PagingCount:                   1,
		AutoDisableCount:              1,
		AutoDisablePublicModelEnabled: true,
	}

	root := &model.User{
		Id:       9401,
		Username: "root-exposure-policy",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(root).Error)
	implementation, ok := model.ResolveLinkImplementation(taskdto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV1,
	})
	require.True(t, ok)
	settingsJSON, err := common.Marshal(taskdto.ChannelOtherSettings{
		VideoUpstreamProfile:           taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamCreatePath:        "/v1/videos",
		VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		AssetUpstreamProfile:           taskdto.AssetUpstreamProfileNone,
		LinkImplementation:             taskdto.LinkImplementationRef{ID: implementation.ID, Version: implementation.Version},
	})
	require.NoError(t, err)
	baseURL := "https://video.example.com"
	modelMapping := `{"seedance-2.0-standard-720p":"seedance-2.0-vip-720p-azhw","unrelated-model":"seedance-2.0-933-720p-azhw"}`
	channel := &model.Channel{
		Id:            9402,
		Type:          constant.ChannelTypeDoubaoVideo,
		Name:          "json-video-policy",
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Models:        model.VideoSKUSeedance20Standard720P + ",unrelated-model",
		Group:         "default",
		BaseURL:       &baseURL,
		ModelMapping:  &modelMapping,
		OtherSettings: string(settingsJSON),
	}
	require.NoError(t, model.DB.Create(channel).Error)
	for _, modelName := range []string{model.VideoSKUSeedance20Standard720P, "unrelated-model"} {
		require.NoError(t, model.DB.Create(&model.Ability{
			Group:     "default",
			Model:     modelName,
			ChannelId: channel.Id,
			Enabled:   true,
		}).Error)
	}
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TaskCreateAttempt{
		AttemptID:            "attempt-policy-exposure",
		PublicTaskID:         "task-policy-exposure",
		UserID:               root.Id,
		ClientProtocol:       model.TaskClientProtocolModelArkV3,
		RequestHash:          "policy-exposure-request",
		ChannelID:            channel.Id,
		PublicModel:          model.VideoSKUSeedance20Standard720P,
		UpstreamProfile:      string(taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays),
		LinkImplementationID: implementation.ID, LinkImplementationVersion: implementation.Version, LinkImplementationHash: implementation.ContentHash,
		Status:           model.TaskCreateAttemptReleasedWithExposure,
		BillingHoldState: model.TaskCreateAttemptBillingReleased,
		OutcomeUnknownAt: now - 20,
		CreatedAt:        now - 10,
		UpdatedAt:        now - 10,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TaskCreateAttempt{
		AttemptID:            "attempt-policy-recovered",
		PublicTaskID:         "task-policy-recovered",
		UserID:               root.Id,
		ClientProtocol:       model.TaskClientProtocolModelArkV3,
		RequestHash:          "policy-recovered-request",
		ChannelID:            channel.Id,
		PublicModel:          model.VideoSKUSeedance20Standard720P,
		UpstreamProfile:      string(taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays),
		LinkImplementationID: implementation.ID, LinkImplementationVersion: implementation.Version, LinkImplementationHash: implementation.ContentHash,
		Status:           model.TaskCreateAttemptComplete,
		BillingHoldState: model.TaskCreateAttemptBillingTransferred,
		OutcomeUnknownAt: now - 20,
		CreatedAt:        now - 20,
		UpdatedAt:        now - 5,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ProviderCostExposure{
		SourceKind:             model.ProviderCostExposureSourceAttempt,
		SourceID:               "attempt-policy-exposure",
		Reason:                 string(model.TaskCreateAttemptReleasedWithExposure),
		UserID:                 root.Id,
		ChannelID:              channel.Id,
		PublicModel:            model.VideoSKUSeedance20Standard720P,
		UpstreamProfile:        string(taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays),
		LinkImplementationID:   implementation.ID,
		LinkImplementationVer:  implementation.Version,
		LinkImplementationHash: implementation.ContentHash,
		CustomerQuotaReleased:  250,
		CreatedAt:              now - 10,
	}).Error)

	assert.Equal(t, 1, EvaluateProviderExposurePolicies(context.Background(), 100))

	var incident model.ProviderExposureIncident
	require.NoError(t, model.DB.First(&incident).Error)
	assert.Equal(t, model.ProviderExposureIncidentOpen, incident.Status)
	assert.Equal(t, model.ProviderExposureSeverityPaging, incident.Severity)
	assert.Equal(t, model.ProviderExposureActionModelDisabled, incident.Action)
	assert.EqualValues(t, 1, incident.ExposureCount)
	assert.EqualValues(t, 250, incident.CustomerQuotaReleased)
	assert.Equal(t, 0, incident.RemainingEquivalentCandidates)
	assert.Equal(t, 0.5, incident.UnknownToExposureRatio)

	var affected, unrelated model.Ability
	require.NoError(t, model.DB.First(&affected,
		"channel_id = ? AND model = ?", channel.Id, model.VideoSKUSeedance20Standard720P).Error)
	require.NoError(t, model.DB.First(&unrelated,
		"channel_id = ? AND model = ?", channel.Id, "unrelated-model").Error)
	assert.False(t, affected.Enabled)
	assert.True(t, unrelated.Enabled)

	assert.Zero(t, EvaluateProviderExposurePolicies(context.Background(), 100))
	var incidentCount int64
	require.NoError(t, model.DB.Model(&model.ProviderExposureIncident{}).Count(&incidentCount).Error)
	assert.EqualValues(t, 1, incidentCount)

	metrics, err := GetProviderExposureMetrics(3600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, metrics.ExposureCount)
	assert.EqualValues(t, 250, metrics.CustomerQuotaReleased)
	require.Len(t, metrics.Metrics, 1)
	assert.Equal(t, 0.5, metrics.Metrics[0].UnknownToExposureRatio)

	resolution, err := ResolveProviderExposureIncident(incident.ID, root.Id, "provider task reconciled", true)
	require.NoError(t, err)
	assert.True(t, resolution.Restored)
	require.NoError(t, model.DB.First(&affected,
		"channel_id = ? AND model = ?", channel.Id, model.VideoSKUSeedance20Standard720P).Error)
	assert.True(t, affected.Enabled)
}

func TestManualTaskCreateAttemptRecoveryRequiresHeldJournalAndIsIdempotent(t *testing.T) {
	truncate(t)
	user := &model.User{
		Id:       9501,
		Username: "manual-attempt-recovery",
		Quota:    100,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		Id:          9502,
		UserId:      user.Id,
		Key:         "manual-attempt-token",
		Name:        "manual-attempt-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 100,
	}
	require.NoError(t, model.DB.Create(token).Error)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:    "task-manual-recovery",
		UserID:          user.Id,
		TokenID:         token.Id,
		ClientProtocol:  model.TaskClientProtocolModelArkV3,
		RequestHash:     "manual-recovery-request",
		ChannelID:       9503,
		PublicModel:     model.VideoSKUSeedance20Standard720P,
		UpstreamProfile: string(taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays),
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID:     attempt.ID,
		FundingSource: BillingSourceWallet,
		ModelName:     model.VideoSKUSeedance20Standard720P,
		Quota:         25,
	})
	require.NoError(t, err)
	template := &model.Task{
		TaskID:         attempt.PublicTaskID,
		Platform:       constant.TaskPlatform("doubao"),
		UserId:         user.Id,
		ChannelId:      attempt.ChannelID,
		Quota:          25,
		Status:         model.TaskStatusQueued,
		Progress:       "0%",
		ClientProtocol: attempt.ClientProtocol,
		Properties: model.Properties{
			OriginModelName: model.VideoSKUSeedance20Standard720P,
		},
		PrivateData: model.TaskPrivateData{
			VideoUpstreamProfile: taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			TokenId:              token.Id,
			BillingSource:        BillingSourceWallet,
		},
	}
	require.NoError(t, model.RecordTaskCreateAttemptRecoveryTemplate(attempt.ID, template))
	require.NoError(t, model.MarkTaskCreateAttemptUnknown(attempt.ID, "request-9501"))

	_, err = RecoverUnknownTaskCreateAttempt(
		attempt.AttemptID,
		"provider-task-9501",
		"",
		false,
		user.Id,
		"provider verification deliberately omitted",
	)
	require.Error(t, err)

	_, err = RecoverUnknownTaskCreateAttempt(
		attempt.AttemptID,
		"provider-task-9501",
		"",
		true,
		user.Id,
		"verified at https://provider.example/task/9501",
	)
	require.Error(t, err)

	recovered, err := RecoverUnknownTaskCreateAttempt(
		attempt.AttemptID,
		"provider-task-9501",
		"request-9501",
		true,
		user.Id,
		"provider console verified",
	)
	require.NoError(t, err)
	assert.Equal(t, attempt.PublicTaskID, recovered.PublicTaskID)
	assert.Equal(t, "provider-task-9501", recovered.UpstreamTaskID)
	assert.Equal(t, string(model.TaskCreateAttemptComplete), recovered.Status)
	assert.Equal(t, string(model.TaskCreateAttemptBillingTransferred), recovered.BillingHoldState)
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, user.Id, attempt.ManualRecoveryBy)
	assert.NotZero(t, attempt.ManualRecoveryAt)
	assert.Equal(t, "provider console verified", attempt.ManualRecoveryNote)
	views, err := model.ListTaskCreateAttemptsForRecovery(model.TaskCreateAttemptComplete, 10, 0)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, attempt.AttemptID, views[0].AttemptID)
	assert.Equal(t, user.Id, views[0].ManualRecoveryBy)

	replayed, err := RecoverUnknownTaskCreateAttempt(
		attempt.AttemptID,
		"provider-task-9501",
		"request-9501",
		true,
		user.Id,
		"idempotent replay",
	)
	require.NoError(t, err)
	assert.Equal(t, recovered.PublicTaskID, replayed.PublicTaskID)
	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).
		Where("task_id = ?", attempt.PublicTaskID).
		Count(&taskCount).Error)
	assert.EqualValues(t, 1, taskCount)

	require.NoError(t, model.DB.First(user, user.Id).Error)
	require.NoError(t, model.DB.First(token, token.Id).Error)
	assert.Equal(t, 75, user.Quota)
	assert.Equal(t, 75, token.RemainQuota)

	releasedAttempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:    "task-manual-recovery-released",
		UserID:          user.Id,
		TokenID:         token.Id,
		ClientProtocol:  model.TaskClientProtocolModelArkV3,
		RequestHash:     "manual-recovery-released-request",
		ChannelID:       9503,
		PublicModel:     model.VideoSKUSeedance20Standard720P,
		UpstreamProfile: string(taskdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays),
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID:     releasedAttempt.ID,
		FundingSource: BillingSourceWallet,
		ModelName:     model.VideoSKUSeedance20Standard720P,
		Quota:         10,
	})
	require.NoError(t, err)
	releasedTemplate := *template
	releasedTemplate.TaskID = releasedAttempt.PublicTaskID
	require.NoError(t, model.RecordTaskCreateAttemptRecoveryTemplate(releasedAttempt.ID, &releasedTemplate))
	require.NoError(t, model.MarkTaskCreateAttemptUnknown(releasedAttempt.ID, "request-released"))
	_, err = model.ReleaseTaskCreateAttemptHold(
		releasedAttempt.ID,
		model.TaskCreateAttemptReleasedWithExposure,
	)
	require.NoError(t, err)
	_, err = RecoverUnknownTaskCreateAttempt(
		releasedAttempt.AttemptID,
		"provider-task-too-late",
		"",
		true,
		user.Id,
		"provider reported a late task",
	)
	require.ErrorIs(t, err, model.ErrTaskCreateAttemptAlreadyReleased)
	var releasedTaskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).
		Where("task_id = ?", releasedAttempt.PublicTaskID).
		Count(&releasedTaskCount).Error)
	assert.Zero(t, releasedTaskCount)
}
