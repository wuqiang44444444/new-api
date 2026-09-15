package controller

import (
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// synlinkVideoSignatureExpired recognizes only explicit TOS signing facts on
// the selected result URL. Unknown/malformed signatures are not expiry evidence.
// This is a content availability check, never a task or billing transition.
func synlinkVideoSignatureExpired(task *model.Task, rawURL string, now time.Time) bool {
	if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	for _, key := range []string{"X-Tos-Algorithm", "X-Tos-Date", "X-Tos-Expires", "X-Tos-Signature"} {
		if len(query[key]) != 1 || query.Get(key) == "" {
			return false
		}
	}
	if query.Get("X-Tos-Algorithm") != "TOS4-HMAC-SHA256" {
		return false
	}
	signedAt, err := time.Parse("20060102T150405Z", query.Get("X-Tos-Date"))
	if err != nil || signedAt.Format("20060102T150405Z") != query.Get("X-Tos-Date") {
		return false
	}
	seconds, err := strconv.ParseUint(query.Get("X-Tos-Expires"), 10, 32)
	if err != nil || seconds == 0 || seconds > 604800 {
		return false
	}
	return !now.Before(signedAt.Add(time.Duration(seconds) * time.Second))
}
