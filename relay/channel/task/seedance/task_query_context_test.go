package seedance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestFunCloudQueryCancellationStopsProviderRequest(t *testing.T) {
	service.InitHttpClient()
	previous := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(previous) })
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	adaptor := &TaskAdaptor{ChannelType: constant.ChannelTypeSeedanceLink}
	task := &model.Task{TaskID: "task-context", PrivateData: model.TaskPrivateData{
		Key: "fixture", VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3,
		VideoUpstreamQueryPathTemplate: "/api/v3/contents/generations/tasks/{task_id}",
		SouthboundAdapterVersion:       relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := adaptor.FetchTaskWithContext(ctx, server.URL, "fixture", task, ""); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	<-stopped
}
