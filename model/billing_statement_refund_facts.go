package model

import (
	"context"
	"sort"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// billingStatementRefundKey identifies one refund evidence candidate. taskID
// holds the Task identity for Task-based classification; preauthID holds the
// manual refund's explicit original-preauth log reference (stage B). A key
// never mixes the two: exactly one of them is non-zero.
type billingStatementRefundKey struct {
	userID, tokenID, channelID int
	model, taskID              string
	preauthID                  int64
}

type billingStatementRefundModes map[billingStatementRefundKey]string

// billingStatementRefundEvidence bundles the two read-only refund fact
// sources: Task-frozen expression classification and the manual-refund
// original-preauth association (quality plan stage B). Both only fill facts
// missing on the refund row; neither changes money or rewrites logs.
type billingStatementRefundEvidence struct {
	modes   billingStatementRefundModes
	preauth billingStatementRefundPreauthFacts
}

func (e billingStatementRefundEvidence) apply(log billingReconciliationLog, parsed *parsedBillingReconciliationLog) {
	if parsed.refundTaskID != "" {
		e.modes.apply(log, parsed)
		return
	}
	if parsed.refundPreauthLogId > 0 {
		e.preauth.apply(log, parsed)
	}
}

// loadBillingStatementRefundEvidence fills only missing classification and
// price evidence on refunds. It never copies prices, amounts, usage or private
// task data into logs. Queries finish before the caller opens its log cursor,
// so separate databases and single-connection SQLite pools both work.
func loadBillingStatementRefundEvidence(ctx context.Context, query *gorm.DB) (billingStatementRefundEvidence, error) {
	var refunds []billingReconciliationLog
	if err := query.Session(&gorm.Session{}).WithContext(ctx).
		Select("user_id, token_id, channel_id, model_name, type, quota, prompt_tokens, completion_tokens, other").
		Where("type = ?", LogTypeRefund).Scan(&refunds).Error; err != nil {
		return billingStatementRefundEvidence{}, err
	}
	parsed := make([]parsedBillingReconciliationLog, len(refunds))
	for i := range refunds {
		parsed[i] = parseBillingReconciliationLog(refunds[i])
	}
	return buildBillingStatementRefundEvidence(ctx, refunds, parsed)
}

func buildBillingStatementRefundEvidence(ctx context.Context, refunds []billingReconciliationLog, parsed []parsedBillingReconciliationLog) (billingStatementRefundEvidence, error) {
	modes, err := buildBillingStatementRefundModes(ctx, refunds, parsed)
	if err != nil {
		return billingStatementRefundEvidence{}, err
	}
	preauth, err := buildBillingStatementRefundPreauthFacts(ctx, refunds, parsed)
	if err != nil {
		return billingStatementRefundEvidence{}, err
	}
	return billingStatementRefundEvidence{modes: modes, preauth: preauth}, nil
}

// buildBillingStatementRefundModes is the bounded-batch variant used by the
// customer export scan: it classifies refunds of one batch only, runs its
// Task lookups under the given context, and holds no state across batches.
func buildBillingStatementRefundModes(ctx context.Context, refunds []billingReconciliationLog, parseds []parsedBillingReconciliationLog) (billingStatementRefundModes, error) {
	type scope struct{ userID, appID int }
	taskIDs := make(map[scope]map[string]struct{})
	modes := make(billingStatementRefundModes)
	for i, log := range refunds {
		parsed := parseds[i]
		if parsed.refundTaskID == "" || log.UserId <= 0 || log.TokenId <= 0 {
			continue
		}
		key := billingStatementRefundKey{userID: log.UserId, tokenID: log.TokenId, channelID: log.ChannelId, model: parsed.customerModel, taskID: parsed.refundTaskID}
		modes[key] = BillingReconciliationModeUnknown
		// Task application identity is frozen from the submitting API key.
		s := scope{log.UserId, log.TokenId}
		if taskIDs[s] == nil {
			taskIDs[s] = make(map[string]struct{})
		}
		taskIDs[s][parsed.refundTaskID] = struct{}{}
	}
	for s, ids := range taskIDs {
		batch := make([]string, 0, len(ids))
		for id := range ids {
			batch = append(batch, id)
		}
		for start := 0; start < len(batch); start += 200 {
			var tasks []Task
			query := DB.WithContext(ctx).Model(&Task{}).
				Select("user_id, app_id, task_id, channel_id, properties, private_data").
				Where("user_id = ? AND app_id = ? AND task_id IN ?", s.userID, s.appID, batch[start:min(start+200, len(batch))])
			if err := query.Find(&tasks).Error; err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, ctxErr
				}
				return nil, err
			}
			seen := make(map[billingStatementRefundKey]bool)
			for _, task := range tasks {
				key := billingStatementRefundKey{userID: task.UserId, tokenID: task.PrivateData.TokenId, channelID: task.ChannelId, model: task.Properties.OriginModelName, taskID: task.TaskID}
				if _, needed := modes[key]; !needed {
					continue
				}
				if seen[key] {
					modes[key] = BillingReconciliationModeUnknown
					continue
				}
				seen[key] = true
				// Match the settlement writer's precedence for frozen snapshots.
				var expression string
				var units map[string]string
				if bc := task.PrivateData.BillingContext; bc != nil && bc.TieredSnapshot != nil {
					expression, units = bc.TieredSnapshot.ExprString, bc.TieredSnapshot.UsageUnits
				}
				if async := task.PrivateData.AsyncBilling; async != nil && async.TieredSnapshot != nil {
					expression, units = async.TieredSnapshot.ExprString, async.TieredSnapshot.UsageUnits
				}
				if expression != "" {
					modes[key] = BillingStatementExpressionMode(expression, units)
				}
			}
		}
	}
	return modes, nil
}

func (modes billingStatementRefundModes) apply(log billingReconciliationLog, parsed *parsedBillingReconciliationLog) {
	if parsed.refundTaskID == "" {
		return
	}
	key := billingStatementRefundKey{userID: log.UserId, tokenID: log.TokenId, channelID: log.ChannelId, model: parsed.customerModel, taskID: parsed.refundTaskID}
	if mode, ok := modes[key]; ok {
		parsed.billingMode = mode
	}
}

// billingStatementPreauthCandidate links one refund row to its explicit
// original-preauth reference with the identity needed for validation.
type billingStatementPreauthCandidate struct {
	key      billingStatementRefundKey
	refQuota int64
}

// billingStatementPreauthOriginal is the minimal original preauth row scanned
// for association validation.
type billingStatementPreauthOriginal struct {
	GroupName string
	Id        int64
	UserId    int
	TokenId   int
	ChannelId int
	ModelName string
	Type      int
	Quota     int
	Other     string
}

// billingStatementRefundPreauthFact carries the customer-safe frozen facts a
// manual refund may inherit from its explicitly referenced original preauth
// consume log: the metering classification and the frozen discount factors.
// Money, tokens and request counts are never inherited.
type billingStatementRefundPreauthFact struct {
	billingMode                     string
	discountRatio                   *float64
	groupName, groupRatioSource     string
	contractDiscountRatio           *float64
	contractEvidence                bool
	contractApplicableKnown         bool
	contractApplicable              bool
	contractId, contractVersion     float64
	contractName                    string
	hasAuxiliaryCharge, unavailable bool
}

type billingStatementRefundPreauthFacts map[billingStatementRefundKey]billingStatementRefundPreauthFact

// apply overlays only facts missing on the refund row. The refund's own
// frozen snapshot always wins; the association never overrides existing
// evidence (quality plan 2.1).
func (facts billingStatementRefundPreauthFacts) apply(log billingReconciliationLog, parsed *parsedBillingReconciliationLog) {
	if parsed.refundPreauthLogId <= 0 {
		return
	}
	key := billingStatementRefundKey{userID: log.UserId, tokenID: log.TokenId, channelID: log.ChannelId, model: parsed.customerModel, preauthID: parsed.refundPreauthLogId}
	fact, ok := facts[key]
	if !ok {
		return
	}
	// Do not assemble a price from mutually inconsistent partial snapshots.
	if (parsed.groupName != "" && fact.groupName != "" && parsed.groupName != fact.groupName) ||
		(parsed.groupRatioSource != "" && fact.groupRatioSource != "" && parsed.groupRatioSource != fact.groupRatioSource) ||
		(parsed.discountRatio != nil && fact.discountRatio != nil && *parsed.discountRatio != *fact.discountRatio) ||
		(parsed.contractDiscountRatio != nil && fact.contractDiscountRatio != nil && *parsed.contractDiscountRatio != *fact.contractDiscountRatio) ||
		billingStatementContractEvidenceConflicts(*parsed, fact) ||
		(parsed.contractId > 0 && fact.contractId > 0 && parsed.contractId != fact.contractId) ||
		(parsed.contractVersion > 0 && fact.contractVersion > 0 && parsed.contractVersion != fact.contractVersion) ||
		(parsed.billingMode != BillingReconciliationModeUnknown && fact.billingMode != BillingReconciliationModeUnknown && parsed.billingMode != fact.billingMode) {
		parsed.unavailable = true
		return
	}
	if parsed.groupName == "" {
		parsed.groupName = fact.groupName
	}
	parsed.hasAuxiliaryCharge = parsed.hasAuxiliaryCharge || fact.hasAuxiliaryCharge
	parsed.unavailable = parsed.unavailable || fact.unavailable
	parsed.contractEvidence = parsed.contractEvidence || fact.contractEvidence
	if parsed.contractId == 0 {
		parsed.contractId = fact.contractId
	}
	if parsed.contractVersion == 0 {
		parsed.contractVersion = fact.contractVersion
	}
	if parsed.contractName == "" {
		parsed.contractName = fact.contractName
	}
	if parsed.billingMode == BillingReconciliationModeUnknown && fact.billingMode != "" && fact.billingMode != BillingReconciliationModeUnknown {
		parsed.billingMode = fact.billingMode
	}
	if parsed.discountRatio == nil {
		parsed.discountRatio = fact.discountRatio
		parsed.groupRatioSource = fact.groupRatioSource
	}
	if parsed.contractDiscountRatio == nil {
		parsed.contractDiscountRatio = fact.contractDiscountRatio
	}
	if !parsed.contractApplicableKnown && fact.contractApplicableKnown {
		parsed.contractApplicableKnown = true
		parsed.contractApplicable = fact.contractApplicable
	}
}

// Positive contract factors prove applicability even when older snapshots omit
// the applicability boolean. Compare semantic evidence before filling fields.
func billingStatementContractEvidenceConflicts(parsed parsedBillingReconciliationLog, fact billingStatementRefundPreauthFact) bool {
	original := parsedBillingReconciliationLog{contractDiscountRatio: fact.contractDiscountRatio,
		contractApplicableKnown: fact.contractApplicableKnown, contractApplicable: fact.contractApplicable}
	left, right := billingContractApplicableState(parsed), billingContractApplicableState(original)
	return (left == combinationContractApplicableYes || left == combinationContractApplicableNo) &&
		(right == combinationContractApplicableYes || right == combinationContractApplicableNo) && left != right
}

// buildBillingStatementRefundPreauthFacts resolves manual refunds whose
// admin_info carries an explicit original_preauth_log_id (stage B). The
// reference must resolve to the exact original consume log with identical
// user, API key, channel and frozen customer model, and this first round only
// covers verified full-amount preauth refunds: one refund per reference and
// the refunded quota equal to the preauth quota. Anything else stays unknown.
func buildBillingStatementRefundPreauthFacts(ctx context.Context, refunds []billingReconciliationLog, parseds []parsedBillingReconciliationLog) (billingStatementRefundPreauthFacts, error) {
	facts := make(billingStatementRefundPreauthFacts)
	if len(refunds) == 0 {
		return facts, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse 持久化身份语义未验收（质量方案 2.4）：无法确认唯一
		// 引用来源时保持未知，不跨库对接数字 ID。
		return facts, nil
	}
	preauthRefs := make(map[int64][]billingStatementPreauthCandidate)
	for i, refund := range refunds {
		if refund.Type != LogTypeRefund || refund.UserId <= 0 || refund.TokenId <= 0 {
			continue
		}
		parsed := parseds[i]
		if parsed.refundTaskID != "" || parsed.refundPreauthLogId <= 0 {
			continue
		}
		key := billingStatementRefundKey{userID: refund.UserId, tokenID: refund.TokenId, channelID: refund.ChannelId, model: parsed.customerModel, preauthID: parsed.refundPreauthLogId}
		preauthRefs[parsed.refundPreauthLogId] = append(preauthRefs[parsed.refundPreauthLogId], billingStatementPreauthCandidate{key: key, refQuota: max(int64(refund.Quota), int64(0))})
	}
	if len(preauthRefs) == 0 {
		return facts, nil
	}
	// Uniqueness belongs to the complete owner/key history, never the
	// current month, page or export batch.
	type scope struct{ userID, tokenID int }
	scopes := make(map[scope][]int64)
	for id, refs := range preauthRefs {
		for _, ref := range refs {
			s := scope{ref.key.userID, ref.key.tokenID}
			scopes[s] = append(scopes[s], id)
		}
	}
	for s, ids := range scopes {
		counts, err := countBillingStatementRefundReferences(ctx, s.userID, s.tokenID, ids)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if counts[id] != 1 {
				delete(preauthRefs, id)
			}
		}
	}
	ids := make([]int64, 0, len(preauthRefs))
	for id := range preauthRefs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for start := 0; start < len(ids); start += 200 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		end := min(start+200, len(ids))
		var originals []billingStatementPreauthOriginal
		query := LOG_DB.WithContext(ctx).Model(&Log{}).
			Select("id, user_id, token_id, channel_id, COALESCE(model_name, '') AS model_name, type, quota, COALESCE(other, '') AS other, "+billingStatementGroupSelect()).
			Where("id IN ?", ids[start:end])
		if err := query.Scan(&originals).Error; err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, err
		}
		for _, original := range originals {
			candidates, ok := preauthRefs[original.Id]
			if !ok {
				continue
			}
			fact, usable := billingStatementPreauthFactFromOriginal(original, candidates)
			if !usable {
				continue
			}
			for _, candidate := range candidates {
				facts[candidate.key] = fact
			}
		}
	}
	return facts, nil
}

// billingStatementPreauthFactFromOriginal validates one original preauth log
// against its referencing refunds and freezes the inheritable facts. Any
// identity mismatch, non-consume reference, shared reference or partial
// refund keeps the association unknown (quality plan 2.1).
func billingStatementPreauthFactFromOriginal(original billingStatementPreauthOriginal, candidates []billingStatementPreauthCandidate) (billingStatementRefundPreauthFact, bool) {
	if original.Type != LogTypeConsume || len(candidates) != 1 {
		return billingStatementRefundPreauthFact{}, false
	}
	candidate := candidates[0]
	if candidate.key.userID != original.UserId || candidate.key.tokenID != original.TokenId ||
		candidate.key.channelID != original.ChannelId || candidate.refQuota != max(int64(original.Quota), int64(0)) {
		return billingStatementRefundPreauthFact{}, false
	}
	parsed := parseBillingReconciliationLog(billingReconciliationLog{
		GroupName: original.GroupName, UserId: original.UserId, TokenId: original.TokenId, TokenName: "",
		ChannelId: original.ChannelId, ModelName: original.ModelName, Type: original.Type,
		PromptTokens: 0, CompletionTokens: 0, Quota: original.Quota, Other: original.Other,
	})
	if parsed.customerModel != candidate.key.model {
		return billingStatementRefundPreauthFact{}, false
	}
	return billingStatementRefundPreauthFact{
		billingMode:   parsed.billingMode,
		discountRatio: parsed.discountRatio,
		groupName:     parsed.groupName, groupRatioSource: parsed.groupRatioSource,
		contractDiscountRatio:   parsed.contractDiscountRatio,
		contractEvidence:        parsed.contractEvidence,
		contractApplicableKnown: parsed.contractApplicableKnown,
		contractApplicable:      parsed.contractApplicable,
		contractId:              parsed.contractId, contractVersion: parsed.contractVersion, contractName: parsed.contractName,
		hasAuxiliaryCharge: parsed.hasAuxiliaryCharge, unavailable: parsed.unavailable,
	}, true
}
