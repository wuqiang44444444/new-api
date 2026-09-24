package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type UpstreamBalanceChannel struct {
	ID       int    `json:"id"`
	KeyIndex int    `json:"key_index"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
}

type UpstreamBalanceConnection struct {
	ID        string                   `json:"id"`
	ChannelID int                      `json:"channel_id"`
	KeyIndex  int                      `json:"key_index"`
	KeyLabel  string                   `json:"key_label"`
	Origin    string                   `json:"origin"`
	URLKey    string                   `json:"url_key"`
	GroupName string                   `json:"group_name"`
	Channels  []UpstreamBalanceChannel `json:"channels"`
	Queryable bool                     `json:"queryable"`
	Reason    string                   `json:"reason,omitempty"`
}

type UpstreamBalanceAmount struct {
	Amount   string `json:"amount"`
	Unit     string `json:"unit"`
	Category string `json:"category,omitempty"`
}

type UpstreamBalanceResult struct {
	Status    string                  `json:"status"`
	Reason    string                  `json:"reason,omitempty"`
	Amounts   []UpstreamBalanceAmount `json:"amounts,omitempty"`
	Scope     string                  `json:"scope,omitempty"`
	CheckedAt int64                   `json:"checked_at"`
}

// Only verified origins have a balance contract. This registry does not affect
// relay routing or infer capabilities from customer model names.
type upstreamBalancePlan struct {
	kind       string
	origin     string
	tokenUsage bool
}

func upstreamBalanceProvider(channel *model.Channel) (upstreamBalancePlan, string, string) {
	base := channel.GetBaseURL()
	if base == "" {
		base = constant.GetChannelBaseURL(channel.Type)
	}
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return upstreamBalancePlan{}, "", "invalid_connection"
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	plan := upstreamBalancePlan{origin: origin}
	// Azure/Vertex credentials are not ordinary Bearer keys even with custom hosts.
	if channel.Type == constant.ChannelTypeAzure || channel.Type == constant.ChannelTypeAzureBatch || channel.Type == constant.ChannelTypeVertexAi {
		return plan, origin, "unsupported_credentials"
	}
	if u.Port() != "" || (u.Scheme != "https" && u.Hostname() != "43.161.200.208") {
		return plan, origin, "not_connected"
	}
	switch strings.ToLower(u.Hostname()) {
	case "mm-internal-cn.leonecloud.com":
		plan.kind = "leone"
	case "mm.leonecloud.com":
		plan.kind, plan.origin = "leone", "https://mm-internal-cn.leonecloud.com"
	case "api.vidu.com":
		plan.kind = "vidu"
	case "www.amutes.com", "us.mixaicloud.com", "synlinkai.com", "43.161.200.208":
		plan.kind, plan.tokenUsage = "newapi", true
	case "tokensave.pro", "www.moxing.pro":
		plan.kind = "newapi"
	case "api.qhaigc.net":
		plan.kind = "qhaigc"
	case "funcloud.ai", "api.funcloud.ai":
		return plan, origin, "unsupported_credentials"
	default:
		return plan, origin, "not_connected"
	}
	return plan, origin, ""
}

// Fingerprints bind results to the saved connection, key and transport settings,
// so editing/reordering credentials cannot attach a result to a stale row.
func upstreamBalanceReference(channel *model.Channel, key string) string {
	setting := ""
	if channel.Setting != nil {
		setting = *channel.Setting
	}
	plan, origin, reason := upstreamBalanceProvider(channel)
	data, _ := common.Marshal([]string{origin, channel.GetBaseURL(), plan.kind, reason, key, setting})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func UpstreamBalanceInventory(channels []*model.Channel) []UpstreamBalanceConnection {
	rows := make([]UpstreamBalanceConnection, 0)
	indices := make(map[string]int)
	for _, channel := range channels {
		keys := []string{channel.Key}
		if channel.ChannelInfo.IsMultiKey {
			keys = channel.GetKeys()
		}
		if len(keys) == 0 {
			keys = []string{""}
		}
		_, origin, reason := upstreamBalanceProvider(channel)
		for keyIndex, key := range keys {
			id := upstreamBalanceReference(channel, key)
			association := UpstreamBalanceChannel{ID: channel.Id, KeyIndex: keyIndex, Name: channel.Name, Enabled: channel.Status == 1 && (!channel.ChannelInfo.IsMultiKey || channel.ChannelInfo.MultiKeyStatusList[keyIndex] <= 1)}
			if index, exists := indices[id]; exists {
				rows[index].Channels = append(rows[index].Channels, association)
				continue
			}
			keyReason := reason
			label := "••••"
			if len(key) >= 12 && !strings.ContainsAny(key, "\r\n{}[]\"") {
				label += key[len(key)-4:]
			}
			if strings.TrimSpace(key) == "" {
				keyReason = "missing_key"
			}
			indices[id] = len(rows)
			rows = append(rows, UpstreamBalanceConnection{ID: id, ChannelID: channel.Id, KeyIndex: keyIndex, KeyLabel: label, Origin: origin, Channels: []UpstreamBalanceChannel{association}, Queryable: keyReason == "", Reason: keyReason})
		}
	}
	return rows
}

var upstreamBalanceSlots = make(chan struct{}, 4)

// QueryUpstreamBalance never calls Channel.GetSetting: malformed settings make
// that legacy helper save the channel, violating this feature's read-only contract.
func QueryUpstreamBalance(ctx context.Context, channel *model.Channel, keyIndex int, reference string) UpstreamBalanceResult {
	result := UpstreamBalanceResult{Status: "error", CheckedAt: time.Now().Unix()}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	if keyIndex < 0 || keyIndex >= len(keys) || reference != upstreamBalanceReference(channel, keys[keyIndex]) {
		result.Reason = "connection_changed"
		return result
	}
	key := strings.TrimSpace(keys[keyIndex])
	plan, _, reason := upstreamBalanceProvider(channel)
	if reason != "" {
		result.Status, result.Reason = "unsupported", reason
		return result
	}
	if key == "" {
		result.Reason = "missing_key"
		return result
	}
	var settings dto.ChannelSettings
	if channel.Setting != nil && *channel.Setting != "" {
		if err := common.UnmarshalJsonStr(*channel.Setting, &settings); err != nil {
			result.Reason = "invalid_connection"
			return result
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	select {
	case upstreamBalanceSlots <- struct{}{}:
		defer func() { <-upstreamBalanceSlots }()
	case <-ctx.Done():
		result.Reason = "timeout"
		return result
	}
	shared, err := GetHttpClientWithProxySettings(settings.Proxy, settings)
	if err != nil {
		result.Reason = "invalid_connection"
		return result
	}
	client := *shared
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	result = queryUpstreamBalanceProvider(ctx, &client, plan, key)
	result.CheckedAt = time.Now().Unix()
	return result
}
