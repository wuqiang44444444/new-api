package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	channelAlertBucketSeconds  = 5
	channelAlertWindow         = 120
	channelAlertThreshold      = 3
	channelAlertMaxScopes      = 512
	channelAlertMaxBatchBytes  = 192 << 10
	channelAlertTTL            = 5 * time.Minute
	channelAlertRule           = 1
	channelAlertClockTolerance = 125 * time.Millisecond
)

type channelAlertKind string

const (
	channelAlertSuccess channelAlertKind = ""
	channelAlertService channelAlertKind = "upstream_service"
	channelAlertNetwork channelAlertKind = "connection"
	channelAlertLimit   channelAlertKind = "rate_limited"
	channelAlertKey     channelAlertKind = "credential"
	channelAlertConfig  channelAlertKind = "configuration"
)

type channelAlertScope struct {
	Channel   int
	Revision  string
	Model     string
	Operation string
	Slot      int
}

type channelAlertSample struct {
	ID     string
	At     int64
	Status int
	Kind   channelAlertKind
}

type channelAlertObservation struct {
	Scope    channelAlertScope
	Success  int64
	Failures []channelAlertSample
}

type channelAlertBatch struct {
	Start    int64
	Instance string
	Members  string
	Scopes   []channelAlertObservation
}

type channelAlertBuffer struct {
	mu      sync.Mutex
	buckets map[int64]map[channelAlertScope]*channelAlertObservation
	dropped map[int64]bool
	sealed  int64
}

type channelAlertStats struct {
	Dropped, Resets, Confirmed, Submitted, Late, Over60 atomic.Uint64
}

var channelAlerts struct {
	once    sync.Once
	enabled atomic.Bool
	buffer  channelAlertBuffer
	hard    chan channelAlertObservation
	wake    chan struct{}
	stats   channelAlertStats
}

func channelAlertBucket(at int64) int64 {
	return at / channelAlertBucketSeconds * channelAlertBucketSeconds
}

func (buffer *channelAlertBuffer) add(scope channelAlertScope, sample channelAlertSample) (bool, bool) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	sample.At = time.Now().UnixMilli()
	bucket := channelAlertBucket(sample.At / 1000)
	if bucket <= buffer.sealed || buffer.dropped[bucket] {
		return false, false
	}
	rows := buffer.buckets[bucket]
	if rows == nil {
		if len(buffer.buckets) >= 3 {
			clear(buffer.buckets)
			clear(buffer.dropped)
			buffer.dropped[bucket] = true
			return false, false
		}
		rows = make(map[channelAlertScope]*channelAlertObservation)
		buffer.buckets[bucket] = rows
	}
	row := rows[scope]
	if row == nil {
		if len(rows) >= channelAlertMaxScopes {
			delete(buffer.buckets, bucket)
			buffer.dropped[bucket] = true
			return false, false
		}
		row = &channelAlertObservation{Scope: scope}
		rows[scope] = row
	}
	if sample.Kind == channelAlertSuccess {
		row.Success = sample.At
		return true, false
	}
	var same int
	first, oldest := -1, -1
	for i, existing := range row.Failures {
		if existing.Kind != sample.Kind {
			continue
		}
		if existing.ID == sample.ID {
			return true, false
		}
		same++
		if first < 0 {
			first = i
			continue
		}
		if oldest < 0 || existing.At < row.Failures[oldest].At {
			oldest = i
		}
	}
	if same > channelAlertThreshold {
		row.Failures[oldest] = sample
	} else {
		row.Failures = append(row.Failures, sample)
	}
	return true, same == 0
}

func (buffer *channelAlertBuffer) seal(start int64) ([]channelAlertObservation, bool) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.sealed = max(buffer.sealed, start)
	rows := buffer.buckets[start]
	dropped := buffer.dropped[start]
	for bucket := range buffer.buckets {
		if bucket <= start {
			delete(buffer.buckets, bucket)
		}
	}
	for bucket := range buffer.dropped {
		if bucket <= start {
			delete(buffer.dropped, bucket)
		}
	}
	observations := make([]channelAlertObservation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, *row)
	}
	return observations, !dropped
}

func recordChannelAlert(scope channelAlertScope, id string, kind channelAlertKind, status int) {
	if !channelAlerts.enabled.Load() || scope.Channel <= 0 || scope.Revision == "" ||
		len(scope.Model) > 128 || len(scope.Operation) > 48 || len(id) > 96 || (kind != channelAlertSuccess && id == "") {
		return
	}
	sample := channelAlertSample{ID: id, Kind: kind, Status: status}
	recorded, first := channelAlerts.buffer.add(scope, sample)
	if !recorded {
		channelAlerts.stats.Dropped.Add(1)
	}
	// Explicit configuration/credential facts do not depend on observing
	// silence in other nodes. The background worker checks their scope.
	if first && (kind == channelAlertConfig || kind == channelAlertKey && scope.Slot < 0) {
		sample.At = time.Now().UnixMilli()
		select {
		case channelAlerts.hard <- channelAlertObservation{Scope: scope, Failures: []channelAlertSample{sample}}:
		default:
			channelAlerts.stats.Dropped.Add(1)
		}
	}
}

func startChannelAlertMonitor() {
	channelAlerts.once.Do(func() {
		channelAlerts.wake = make(chan struct{}, 1)
		channelAlerts.buffer.buckets = make(map[int64]map[channelAlertScope]*channelAlertObservation)
		channelAlerts.buffer.dropped = make(map[int64]bool)
		channelAlerts.hard = make(chan channelAlertObservation, 128)
		instance := common.GetRandomString(24)
		namespace := hmac.New(sha256.New, []byte(common.CryptoSecret))
		_, _ = namespace.Write([]byte("channel-alert:v2"))
		cluster := hex.EncodeToString(namespace.Sum(nil))

		channelAlerts.enabled.Store(true)
		go runDefiniteChannelAlerts()
		if common.IsMasterNode {
			go runChannelAlertDelivery()
		}
		if !common.RedisEnabled || common.RDB == nil {
			go runChannelAlertEvaluator(cluster, instance)
			return
		}
		go publishChannelAlertBatches(common.NodeName, instance, cluster)
		if common.IsMasterNode {
			go runChannelAlertEvaluator(cluster, "")
		}
	})
}

func channelAlertDigest(fields ...string) string {
	hash := sha256.New()
	for _, field := range fields {
		fmt.Fprintf(hash, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func channelAlertBatchKey(members, node string, start int64) string {
	return "channel-alert:v2:" + members[:16] + ":" + node + ":" + strconv.FormatInt(start, 10)
}

func publishChannelAlertBatches(node, instance, members string) {
	start := channelAlertBucket(time.Now().Unix()) + channelAlertBucketSeconds
	for {
		time.Sleep(time.Until(time.Unix(start+channelAlertBucketSeconds, 0)))
		now := channelAlertBucket(time.Now().Unix())
		latest := now - channelAlertBucketSeconds
		if latest < start {
			continue
		}
		rows, valid := channelAlerts.buffer.seal(latest)
		if valid {
			batch := channelAlertBatch{Start: latest, Instance: instance, Members: members, Scopes: rows}
			payload, err := common.Marshal(batch)
			if err == nil && len(payload) <= channelAlertMaxBatchBytes {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				saved := writeChannelAlertBatch(ctx, channelAlertBatchKey(members, node, latest), payload)
				cancel()
				if !saved {
					channelAlerts.stats.Dropped.Add(1)
				}
			} else {
				channelAlerts.stats.Dropped.Add(1)
			}
		}
		start = now
	}
}

// An out-of-sync instance leaves a missing batch instead of publishing
// observations whose ordering cannot be compared with the other nodes.
func writeChannelAlertBatch(ctx context.Context, key string, payload []byte) bool {
	before := time.Now()
	redisTime, err := common.RDB.Time(ctx).Result()
	roundTrip := time.Since(before)
	if err != nil {
		return false
	}
	skew := redisTime.Sub(before.Add(roundTrip / 2))
	if skew.Abs()+roundTrip/2 > channelAlertClockTolerance {
		return false
	}
	saved, err := common.RDB.SetNX(ctx, key, payload, channelAlertTTL).Result()
	return err == nil && saved
}
