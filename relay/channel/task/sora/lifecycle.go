package sora

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
)

func (*TaskAdaptor) TaskLifecycleCapabilities(_ *model.Task) channel.TaskLifecycleCapabilities {
	return channel.TaskLifecycleCapabilities{
		SupportsContent:        true,
		SupportsRemix:          true,
		SupportsDeleteTerminal: true,
	}
}

func (*TaskAdaptor) CancelQueuedTask(context.Context, *model.Task, *model.Channel) error {
	return &channel.TaskLifecycleError{StatusCode: http.StatusConflict, Message: "queued cancellation is not supported"}
}

func (*TaskAdaptor) DeleteTerminalTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
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
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/videos/" + url.PathEscape(task.GetUpstreamTaskID())
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
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
