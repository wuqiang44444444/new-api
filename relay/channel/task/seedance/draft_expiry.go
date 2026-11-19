package seedance

import (
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func VideoDraftExpired(task *model.Task, now time.Time) bool {
	if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolViduModelArkV3 {
		return false
	}
	base, err := url.Parse(task.PrivateData.VideoUpstreamQueryBaseURL)
	// Apply the domestic policy only to a recognized, frozen domestic endpoint.
	if err != nil || base.Scheme != "https" || !strings.EqualFold(base.Hostname(), "api.vidu.cn") ||
		(base.Port() != "" && base.Port() != "443") || strings.TrimSuffix(base.EscapedPath(), "/") != "/ent" {
		return false
	}
	var observation struct {
		CreatedAt int64 `json:"created_at"`
	}
	if common.Unmarshal(task.Data, &observation) != nil || observation.CreatedAt <= 0 {
		return false
	}
	created := time.Unix(observation.CreatedAt, 0)
	cutover := time.Date(2026, time.October, 28, 2, 0, 0, 0, time.UTC)
	days := 14
	if created.Before(cutover) {
		days = 7
	}
	return !now.Before(created.Add(time.Duration(days) * 24 * time.Hour))
}
