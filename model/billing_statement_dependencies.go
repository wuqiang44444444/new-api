package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// The task identity matches the existing refund evidence query, including absent
// matches. A digest keeps opaque provider identifiers within the scope column.
func billingStatementTaskScope(user, app int, task string) string {
	return fmt.Sprintf("task:%d:%d:%x", user, app, sha256.Sum256([]byte(task)))
}

// Only external facts actually consulted by this month's refund parser belong
// in the vector. The month revision, read before this scan, guards its membership.
func billingStatementEvidenceScopes(ctx context.Context, user int, start int64, policies ...BillingStatementReadPolicy) ([]string, error) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	end := time.Unix(start, 0).In(loc).AddDate(0, 1, 0).Unix()
	policy := BillingStatementReadPolicy{BatchTimeout: 5 * time.Second, MaxGroups: 10000}
	if len(policies) > 0 {
		policy = policies[0]
	}
	scopes := map[string]struct{}{}
	cursor := int64(0)
	for {
		if policy.BeforeBatch != nil {
			if err := policy.BeforeBatch(ctx); err != nil {
				return nil, err
			}
		}
		batchCtx := ctx
		cancel := func() {}
		if policy.BatchTimeout > 0 {
			batchCtx, cancel = context.WithTimeout(ctx, policy.BatchTimeout)
		}
		var logs []Log
		err := LOG_DB.WithContext(batchCtx).Model(&Log{}).
			Select("id, user_id, token_id, channel_id, model_name, type, quota, prompt_tokens, completion_tokens, other").
			Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ? AND id > ?", user, LogTypeRefund, start, end, cursor).
			Order("id asc").Limit(500).Find(&logs).Error
		cancel()
		if err != nil {
			return nil, err
		}
		if len(logs) == 0 {
			break
		}
		for _, log := range logs {
			fact := billingReconciliationLog{UserId: log.UserId, TokenId: log.TokenId, ChannelId: log.ChannelId, ModelName: log.ModelName, Type: log.Type, Quota: log.Quota, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Other: log.Other}
			parsed := parseBillingReconciliationLog(fact)
			if parsed.refundTaskID != "" && log.TokenId > 0 {
				scopes[billingStatementTaskScope(user, log.TokenId, parsed.refundTaskID)] = struct{}{}
			} else if parsed.refundPreauthLogId > 0 {
				scopes[fmt.Sprintf("log:%d", parsed.refundPreauthLogId)] = struct{}{}
				scopes[fmt.Sprintf("refund:%d:%d", user, log.TokenId)] = struct{}{}
			}
			if len(scopes) > 20000 {
				return nil, fmt.Errorf("statement evidence budget exceeded")
			}
		}
		cursor = int64(logs[len(logs)-1].Id)
	}
	result := make([]string, 0, len(scopes))
	for scope := range scopes {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result, nil
}

func billingStatementDependencies(v *BillingStatementVersion) (map[string]int64, error) {
	var dependencies map[string]int64
	if err := common.UnmarshalJsonStr(string(v.Dependencies), &dependencies); err != nil {
		return nil, err
	}
	if dependencies == nil {
		return nil, ErrBillingStatementVersionConflict
	}
	return dependencies, nil
}

// The caller supplies a locked transaction for confirmation, or a read-only
// query for generation. Bounded IN queries avoid one round trip per dependency.
func verifyBillingStatementEvidenceRevisions(query *gorm.DB, v *BillingStatementVersion) error {
	dependencies, err := billingStatementDependencies(v)
	if err != nil {
		return err
	}
	scopes := make([]string, 0, len(dependencies))
	for scope := range dependencies {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for start := 0; start < len(scopes); start += 500 {
		batch := scopes[start:min(start+500, len(scopes))]
		var revisions []BillingStatementRevision
		if err := query.Session(&gorm.Session{}).Select("scope, revision").Where("scope IN ?", batch).Order("scope asc").Find(&revisions).Error; err != nil {
			return err
		}
		if len(revisions) != len(batch) {
			return ErrBillingStatementVersionConflict
		}
		for _, revision := range revisions {
			if revision.Revision != dependencies[revision.Scope] {
				return ErrBillingStatementVersionConflict
			}
		}
	}
	return nil
}
