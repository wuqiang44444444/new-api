package doubao

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
)

func (*TaskAdaptor) TaskLifecycleCapabilities(task *model.Task) channel.TaskLifecycleCapabilities {
	official := task != nil && (task.PrivateData.VideoUpstreamProfile == "" || task.PrivateData.VideoUpstreamProfile == dto.VideoUpstreamProfileOfficial)
	return channel.TaskLifecycleCapabilities{
		SupportsContent:         true,
		SupportsCancelQueued:    official,
		SupportsDeleteTerminal:  official,
		SupportsAssetReferences: true,
	}
}

func (a *TaskAdaptor) CancelQueuedTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
	return a.deleteOfficialTask(ctx, task, providerChannel)
}

func (a *TaskAdaptor) DeleteTerminalTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
	return a.deleteOfficialTask(ctx, task, providerChannel)
}

func (*TaskAdaptor) deleteOfficialTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
	baseURL := task.PrivateData.VideoUpstreamQueryBaseURL
	key := task.PrivateData.Key
	proxy := task.PrivateData.VideoUpstreamProxy
	frozen := model.TaskUsesFrozenVideoConnection(task)
	if frozen && (baseURL == "" || key == "") {
		return fmt.Errorf("frozen upstream connection details are unavailable")
	}
	if providerChannel != nil {
		if baseURL == "" {
			baseURL = providerChannel.GetBaseURL()
		}
		if key == "" {
			key = providerChannel.Key
		}
		if !frozen {
			proxy = providerChannel.GetSetting().Proxy
		}
	}
	path, err := videoTaskPath(dto.VideoUpstreamProfileOfficial, "", task.GetUpstreamTaskID())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, joinVideoUpstreamURL(baseURL, path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &channel.TaskLifecycleError{StatusCode: resp.StatusCode}
	}
	return nil
}
