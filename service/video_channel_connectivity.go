package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const officialVideoConnectivityPath = "/api/v3/contents/generations/tasks?page_size=1"

func CheckVideoChannelConnectivity(ctx context.Context, channel *model.Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return newChannelConnectivityError(
			ChannelConnectivityVideoInvalidConfig,
			"channel is not a Seedance Link channel",
			nil,
		)
	}
	protocol := channel.GetOtherSettings().VideoUpstreamProtocol
	if protocol != dto.VideoUpstreamProtocolModelArkV3Volcengine && protocol != dto.VideoUpstreamProtocolModelArkV3BytePlus {
		return newChannelConnectivityError(
			ChannelConnectivityVideoInvalidConfig,
			"Seedance channel does not use an official ModelArk V3 protocol",
			nil,
		)
	}
	baseURL := strings.TrimSpace(channel.GetBaseURL())
	apiKey := strings.TrimSpace(channel.Key)
	if baseURL == "" || apiKey == "" {
		return newChannelConnectivityError(
			ChannelConnectivityVideoNotConfigured,
			"official video API credentials are not configured",
			nil,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimRight(baseURL, "/")+officialVideoConnectivityPath,
		nil,
	)
	if err != nil {
		return newChannelConnectivityError(
			ChannelConnectivityVideoInvalidConfig,
			"official video API configuration is invalid",
			err,
		)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return newChannelConnectivityError(
			ChannelConnectivityVideoInvalidConfig,
			"official video API configuration is invalid",
			err,
		)
	}
	resp, err := client.Do(req)
	if err != nil {
		return newChannelConnectivityError(
			ChannelConnectivityVideoUnavailable,
			"official video API is unavailable",
			err,
		)
	}
	if resp == nil {
		return newChannelConnectivityError(
			ChannelConnectivityVideoUnavailable,
			"official video API is unavailable",
			errors.New("empty upstream response"),
		)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	cause := fmt.Errorf("official video API returned HTTP %d", resp.StatusCode)
	if resp.StatusCode >= http.StatusBadRequest &&
		resp.StatusCode < http.StatusInternalServerError &&
		resp.StatusCode != http.StatusRequestTimeout &&
		resp.StatusCode != http.StatusTooEarly &&
		resp.StatusCode != http.StatusTooManyRequests {
		return newChannelConnectivityError(
			ChannelConnectivityVideoRejected,
			"official video API rejected the request",
			cause,
		)
	}
	return newChannelConnectivityError(
		ChannelConnectivityVideoUnavailable,
		"official video API is unavailable",
		cause,
	)
}
