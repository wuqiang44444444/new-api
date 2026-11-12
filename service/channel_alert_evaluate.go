package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type channelAlertRound struct {
	start   int64
	batches []channelAlertBatch
}

type channelAlertWindowState struct {
	localNode string
	rounds    []channelAlertRound
	bytes     int
	sizes     []int
}

func (window *channelAlertWindowState) reset() {
	window.rounds = nil
	window.sizes = nil
	window.bytes = 0
	channelAlerts.stats.Resets.Add(1)
}

func (window *channelAlertWindowState) append(round channelAlertRound, size int) {
	const maxBytes = 8 << 20
	limit := channelAlertWindow / channelAlertBucketSeconds
	if len(window.rounds) > 0 {
		last := window.rounds[len(window.rounds)-1]
		if last.start+channelAlertBucketSeconds != round.start {
			window.reset()
		} else {
			for i, batch := range last.batches {
				if round.batches[i].Instance != batch.Instance {
					window.reset()
					break
				}
			}
		}
	}
	for len(window.rounds) > 0 && (len(window.rounds) >= limit || window.bytes+size > maxBytes) {
		if window.bytes+size > maxBytes {
			window.reset()
			break
		}
		window.bytes -= window.sizes[0]
		window.rounds[0] = channelAlertRound{}
		window.rounds = window.rounds[1:]
		window.sizes = window.sizes[1:]
	}
	if size <= maxBytes {
		window.bytes += size
		window.sizes = append(window.sizes, size)
		window.rounds = append(window.rounds, round)
	} else {
		window.reset()
	}
}

func readChannelAlertRound(ctx context.Context, nodes []string, members string, start int64, window *channelAlertWindowState) (channelAlertRound, int, bool) {
	keys := make([]string, len(nodes))
	for i, node := range nodes {
		keys[i] = channelAlertBatchKey(members, node, start)
	}
	var previous []string
	for _, round := range window.rounds {
		for _, node := range nodes {
			previous = append(previous, channelAlertBatchKey(members, node, round.start))
		}
	}
	var results []any
	var err error
	if len(previous) == 0 {
		results, err = common.RDB.MGet(ctx, keys...).Result()
	} else {
		pipeline := common.RDB.TxPipeline()
		present := pipeline.Exists(ctx, previous...)
		result := pipeline.MGet(ctx, keys...)
		_, err = pipeline.Exec(ctx)
		if err == nil && present.Val() != int64(len(previous)) {
			window.reset()
		}
		results = result.Val()
	}
	if err != nil || len(results) != len(nodes) {
		return channelAlertRound{}, 0, false
	}
	round := channelAlertRound{start: start, batches: make([]channelAlertBatch, len(nodes))}
	size := 0
	for i, raw := range results {
		payload, ok := raw.(string)
		batch := &round.batches[i]
		if !ok || len(payload) > channelAlertMaxBatchBytes || common.UnmarshalJsonStr(payload, batch) != nil ||
			batch.Start != start || batch.Members != members || batch.Instance == "" {
			return channelAlertRound{}, 0, false
		}
		size += len(payload)
	}
	return round, size, true
}

type channelAlertGroup struct {
	scope channelAlertScope
	node  string
	kind  channelAlertKind
}

type channelAlertWindowEvidence struct {
	successes map[channelAlertGroup]int64
	failures  map[channelAlertGroup][]channelAlertSample
	slots     map[channelAlertScope]map[int]int64
	localNode string
}

func (window channelAlertWindowState) evidence(nodes []string) channelAlertWindowEvidence {
	evidence := channelAlertWindowEvidence{
		successes: make(map[channelAlertGroup]int64),
		failures:  make(map[channelAlertGroup][]channelAlertSample),
		slots:     make(map[channelAlertScope]map[int]int64),
		localNode: window.localNode,
	}
	cutoff := (window.rounds[len(window.rounds)-1].start+channelAlertBucketSeconds)*1000 - (2 * channelAlertClockTolerance).Milliseconds()
	for _, round := range window.rounds {
		for i, batch := range round.batches {
			for _, observation := range batch.Scopes {
				scope := observation.Scope
				scope.Slot = -1
				group := channelAlertGroup{scope: scope}
				evidence.successes[group] = max(evidence.successes[group], observation.Success)
				group.node = nodes[i]
				evidence.successes[group] = max(evidence.successes[group], observation.Success)
				if observation.Success > 0 && observation.Scope.Slot >= 0 {
					slots := evidence.slots[scope]
					if slots == nil {
						slots = make(map[int]int64)
						evidence.slots[scope] = slots
					}
					slots[observation.Scope.Slot] = max(slots[observation.Scope.Slot], observation.Success)
				}
				for _, sample := range observation.Failures {
					if sample.At > cutoff {
						continue
					}
					group = channelAlertGroup{scope: scope, kind: sample.Kind}
					if sample.Kind != channelAlertConfig {
						group.node = window.localNode
					}
					switch sample.Kind {
					case channelAlertNetwork:
						group.node = nodes[i]
					case channelAlertKey:
						group.scope.Slot = observation.Scope.Slot
					}
					evidence.failures[group] = append(evidence.failures[group], sample)
				}
			}
		}
	}
	return evidence
}

func channelAlertCandidate(group channelAlertGroup, samples []channelAlertSample, success int64) (model.ChannelAlertEpisode, bool) {
	eligible := make(map[string]channelAlertSample, len(samples))
	for _, sample := range samples {
		if group.kind != channelAlertLimit && sample.At <= success {
			continue
		}
		if previous, found := eligible[sample.ID]; !found || sample.At < previous.At {
			eligible[sample.ID] = sample
		}
	}
	threshold := channelAlertThreshold
	if group.kind == channelAlertKey || group.kind == channelAlertConfig {
		threshold = 1
	}
	if len(eligible) < threshold || (group.kind == channelAlertLimit && success == 0) {
		return model.ChannelAlertEpisode{}, false
	}
	var first, last int64
	status := 0
	for _, sample := range eligible {
		if first == 0 || sample.At < first {
			first = sample.At
		}
		if sample.At > last {
			last, status = sample.At, sample.Status
		}
	}
	scope := group.scope
	kind := string(group.kind)
	key := channelAlertDigest(strconv.Itoa(scope.Channel), scope.Revision, scope.Model, scope.Operation, strconv.Itoa(scope.Slot), group.node, kind)
	return model.ChannelAlertEpisode{
		OpenKey: &key, ScopeKey: key, ChannelID: scope.Channel, Revision: scope.Revision, Model: scope.Model, Operation: scope.Operation,
		Node: group.node, Kind: kind, KeySlot: scope.Slot, StatusCode: status, FirstAt: first, LastAt: last, EvidenceCount: len(eligible), RuleVersion: channelAlertRule,
	}, true
}

func runChannelAlertEvaluator(cluster, localInstance string) {
	window := channelAlertWindowState{}
	var nodes []string
	if localInstance != "" {
		window.localNode = common.NodeName
		nodes = []string{common.NodeName}
	}
	lastPurge := time.Now()
	for {
		now := time.Now()
		target := channelAlertBucket(now.Unix()) + channelAlertBucketSeconds
		time.Sleep(time.Until(time.Unix(target, 0).Add(2 * time.Second)))
		now = time.Now()
		start := channelAlertBucket(now.Unix()) - channelAlertBucketSeconds
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		var round channelAlertRound
		var size int
		var valid bool
		if localInstance != "" {
			round, size, valid = sealLocalChannelAlertRound(start, localInstance)
		} else {
			var active []string
			active, valid = activeChannelAlertNodes(ctx, now)
			if valid {
				if !slices.Equal(nodes, active) {
					window.reset()
					nodes = active
				}
				round, size, valid = readChannelAlertRound(ctx, nodes, cluster, start, &window)
			}
		}
		cancel()
		if !valid {
			window.reset()
			continue
		}
		window.append(round, size)
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		if err := evaluateChannelAlertWindow(ctx, window, nodes, now); err != nil {
			common.SysError("channel alert evaluation failed")
		}
		if common.IsMasterNode && time.Since(lastPurge) >= time.Hour {
			lastPurge = time.Now()
			if err := model.PurgeClosedChannelAlerts(ctx, lastPurge.Add(-30*24*time.Hour).UnixMilli()); err != nil {
				common.SysError("channel alert retention failed")
			}
		}
		cancel()
	}
}

func activeChannelAlertNodes(ctx context.Context, now time.Time) ([]string, bool) {
	const maxNodes = 32
	var nodes []string
	err := model.DB.WithContext(ctx).Model(&model.SystemInstance{}).
		Where("last_seen_at >= ?", now.Unix()-model.SystemInstanceStaleAfterSeconds).
		Order("node_name").Limit(maxNodes+1).Pluck("node_name", &nodes).Error
	if err != nil || len(nodes) > maxNodes || !slices.Contains(nodes, common.NodeName) {
		return nil, false
	}
	return nodes, true
}

func sealLocalChannelAlertRound(start int64, instance string) (channelAlertRound, int, bool) {
	rows, valid := channelAlerts.buffer.seal(start)
	if !valid {
		return channelAlertRound{}, 0, false
	}
	batch := channelAlertBatch{Start: start, Instance: instance, Scopes: rows}
	payload, err := common.Marshal(batch)
	if err != nil || len(payload) > channelAlertMaxBatchBytes {
		channelAlerts.stats.Dropped.Add(1)
		return channelAlertRound{}, 0, false
	}
	return channelAlertRound{start: start, batches: []channelAlertBatch{batch}}, len(payload), true
}

func evaluateChannelAlertWindow(ctx context.Context, window channelAlertWindowState, nodes []string, now time.Time) error {
	if len(window.rounds) == 0 {
		return nil
	}
	evidence := window.evidence(nodes)
	var open []model.ChannelAlertEpisode
	if err := model.DB.WithContext(ctx).Where("closed_at = 0 AND open_key IS NOT NULL AND (preview = ? OR preview IS NULL)", false).Find(&open).Error; err != nil {
		return err
	}
	opened := make(map[string]bool, len(open))
	channels := make(map[int]*model.Channel)
	deleted := make(map[int]bool)
	// Only channels with an open or candidate incident need a current DB read.
	channelFor := func(id int) *model.Channel {
		if channel, found := channels[id]; found {
			return channel
		}
		var channel model.Channel
		if err := model.DB.WithContext(ctx).First(&channel, id).Error; err != nil {
			deleted[id] = errors.Is(err, gorm.ErrRecordNotFound)
			channels[id] = nil
			return nil
		}
		channels[id] = &channel
		return &channel
	}
	for _, event := range open {
		if window.localNode != "" && event.Node != window.localNode &&
			(event.Node != "" || (event.Kind != string(channelAlertKey) && event.Kind != string(channelAlertConfig))) {
			continue
		}
		scope := channelAlertScope{Channel: event.ChannelID, Revision: event.Revision, Model: event.Model, Operation: event.Operation, Slot: -1}
		group := channelAlertGroup{scope: scope, node: event.Node}
		success := evidence.successes[group]
		closeReason := model.ChannelAlertClosedRecovered
		switch channelAlertKind(event.Kind) {
		case channelAlertKey:
			if event.KeySlot >= 0 {
				success = evidence.slots[scope][event.KeySlot]
			}
		case channelAlertLimit:
			group.kind = channelAlertLimit
			if len(window.rounds) < channelAlertWindow/channelAlertBucketSeconds || len(evidence.failures[group]) > 0 {
				success = 0
			}
		}
		if channel := channelFor(event.ChannelID); channel != nil {
			revision, err := model.CurrentChannelAlertRevision(model.DB.WithContext(ctx), channel, event.Operation, event.Revision)
			if err != nil {
				return err
			}
			if revision != event.Revision {
				success = now.UnixMilli()
				closeReason = model.ChannelAlertClosedConfigChanged
			}
		} else if deleted[event.ChannelID] {
			success = now.UnixMilli()
			closeReason = model.ChannelAlertClosedDeleted
		}
		lastFailure := event.LastAt
		if closeReason == model.ChannelAlertClosedRecovered && event.Node == "" {
			lastFailure += (2 * channelAlertClockTolerance).Milliseconds()
		}
		if success > lastFailure {
			if err := model.CloseChannelAlert(ctx, event.ID, success, closeReason); err != nil {
				return err
			}
		} else {
			opened[event.ScopeKey] = true
		}
	}
	for group, samples := range evidence.failures {
		if group.kind == channelAlertKey {
			continue
		}
		success := evidence.successes[channelAlertGroup{scope: group.scope, node: group.node}]
		// Small cross-node skew must not turn a near-simultaneous success into silence.
		if success > 0 && group.node == "" {
			success += (2 * channelAlertClockTolerance).Milliseconds()
		}
		episode, confirmed := channelAlertCandidate(group, samples, success)
		if confirmed && !opened[episode.ScopeKey] {
			if err := confirmChannelAlert(ctx, episode, now); err != nil {
				return err
			}
		}
	}
	return evaluateChannelAlertKeys(ctx, evidence, channelFor, opened, now)
}

func evaluateChannelAlertKeys(ctx context.Context, evidence channelAlertWindowEvidence, channelFor func(int) *model.Channel, opened map[string]bool, now time.Time) error {
	invalid := make(map[channelAlertScope]map[int]channelAlertSample)
	for group, failures := range evidence.failures {
		if group.kind != channelAlertKey || group.scope.Slot < 0 {
			continue
		}
		scope := group.scope
		slot := scope.Slot
		scope.Slot = -1
		for _, failure := range failures {
			if failure.At <= evidence.slots[scope][slot] || failure.At <= invalid[scope][slot].At {
				continue
			}
			if invalid[scope] == nil {
				invalid[scope] = make(map[int]channelAlertSample)
			}
			invalid[scope][slot] = failure
		}
	}
	for scope, slots := range invalid {
		channel := channelFor(scope.Channel)
		if channel == nil || model.ChannelAlertRevision(channel) != scope.Revision || !channel.ChannelInfo.IsMultiKey {
			continue
		}
		var latest int64
		var failed []channelAlertSample
		credentials := model.ChannelAlertCredentialSlots(channel)
		for _, index := range credentials {
			if sample, failedSlot := slots[index]; failedSlot {
				failed = append(failed, sample)
			} else if channel.ChannelInfo.MultiKeyStatusList[index] != common.ChannelStatusAutoDisabled {
				latest = max(latest, evidence.slots[scope][index])
			}
		}
		if len(credentials) > 0 && len(credentials) == len(failed) {
			episode, ok := channelAlertCandidate(channelAlertGroup{scope: scope, kind: channelAlertKey, node: evidence.localNode}, failed, 0)
			episode.CredentialSlots = credentials
			if ok && !opened[episode.ScopeKey] {
				if err := confirmChannelAlert(ctx, episode, now); err != nil {
					return err
				}
			}
		} else if latest > 0 {
			for _, index := range credentials {
				sample, failedSlot := slots[index]
				if !failedSlot {
					continue
				}
				keyScope := scope
				keyScope.Slot = index
				episode, ok := channelAlertCandidate(channelAlertGroup{scope: keyScope, kind: channelAlertKey, node: evidence.localNode}, []channelAlertSample{sample}, 0)
				episode.CredentialSlots = credentials
				if ok && !opened[episode.ScopeKey] {
					if err := confirmChannelAlert(ctx, episode, now); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func runDefiniteChannelAlerts() {
	for observation := range channelAlerts.hard {
		sample := observation.Failures[0]
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		episode, ok := channelAlertCandidate(channelAlertGroup{scope: observation.Scope, kind: sample.Kind}, observation.Failures, 0)
		if ok {
			if err := confirmChannelAlert(ctx, episode, time.Now()); err != nil {
				common.SysError("channel alert confirmation failed")
			}
		}
		cancel()
	}
}

func confirmChannelAlert(ctx context.Context, episode model.ChannelAlertEpisode, now time.Time) error {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return err
	}
	episode.ConfirmedAt = now.UnixMilli()
	opened, err := model.OpenChannelAlert(ctx, episode, recipients)
	if opened {
		channelAlerts.stats.Confirmed.Add(1)
		common.SysLog(fmt.Sprintf("channel alert confirmed channel_id=%d kind=%s observation_ms=%d processing_ms=%d", episode.ChannelID, episode.Kind, episode.ConfirmedAt-episode.FirstAt, episode.ConfirmedAt-episode.LastAt))
		select {
		case channelAlerts.wake <- struct{}{}:
		default:
		}
	}
	return err
}
