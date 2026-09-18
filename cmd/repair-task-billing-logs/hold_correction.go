package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// This is a log projection correction, never a funding or usage-counter action.
// Plans contain row identities and evidence digests, not private Task snapshots.
type holdCorrection struct {
	TaskID   int64  `json:"task_row_id"`
	UserID   int    `json:"user_id"`
	LogID    int    `json:"log_id"`
	Period   int64  `json:"period_start"`
	Quota    int64  `json:"missing_quota"`
	Evidence string `json:"evidence_sha256"`
	Applied  bool   `json:"applied"`
}
type holdGap struct {
	TaskID int64  `json:"task_row_id"`
	UserID int    `json:"user_id"`
	Gap    int64  `json:"gap_quota"`
	Reason string `json:"reason"`
}
type holdCorrectionPlan struct {
	Version    int              `json:"version"`
	CapturedAt int64            `json:"captured_at"`
	MaxLogID   int              `json:"max_log_id"`
	Candidates []holdCorrection `json:"candidates"`
	Unresolved []holdGap        `json:"unresolved"`
}
type holdLog struct {
	ID        int
	UserID    int
	TokenID   int
	ChannelID int
	ModelName string
	Type      int
	Quota     int64
	CreatedAt int64
	Other     string
}

// Read all current evidence in the caller's consistent transaction. Complete
// history is required: a later refund can disprove an apparently missing hold.
func auditHeldTaskProjection(tx *gorm.DB) (*holdCorrectionPlan, error) {
	var tasks []model.Task
	if err := tx.Select("id,task_id,user_id,app_id,channel_id,quota,properties,private_data,billing_state,status").Order("id").Find(&tasks).Error; err != nil {
		return nil, err
	}
	var attempts []model.TaskCreateAttempt
	if err := tx.Select("id,public_task_id,user_id,token_id,app_id,channel_id,public_model,status,billing_hold_state,billing_source,held_quota").Order("id").Find(&attempts).Error; err != nil {
		return nil, err
	}
	byTask := map[string][]model.TaskCreateAttempt{}
	for _, a := range attempts {
		byTask[a.PublicTaskID] = append(byTask[a.PublicTaskID], a)
	}
	logs := map[string][]holdLog{}
	refs := map[int][]holdLog{}
	owners := map[string]int{}
	unreadable := map[string]bool{}
	for _, t := range tasks {
		owners[t.TaskID]++
	}
	var maxID int
	if err := tx.Model(&model.Log{}).Select("COALESCE(MAX(id),0)").Scan(&maxID).Error; err != nil {
		return nil, err
	}
	for cursor := 0; cursor < maxID; {
		var batch []holdLog
		if err := tx.Table("logs").Select("id,user_id,token_id,channel_id,model_name,type,quota,created_at,other").Where("id>? AND id<=? AND type IN ?", cursor, maxID, []int{model.LogTypeConsume, model.LogTypeRefund}).Order("id").Limit(500).Scan(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, l := range batch {
			var other map[string]any
			if common.UnmarshalJsonStr(l.Other, &other) != nil || other == nil {
				unreadable[fmt.Sprintf("%d:%d:%d:%s", l.UserID, l.TokenID, l.ChannelID, l.ModelName)] = true
				continue
			}
			taskID, _ := other["task_id"].(string)
			if owners[taskID] > 0 {
				logs[taskID] = append(logs[taskID], l)
			}
			// Decode the numeric reference strictly; floats must not round large IDs.
			var ref struct {
				Admin struct {
					ID int `json:"original_preauth_log_id"`
				} `json:"admin_info"`
			}
			if l.Type == model.LogTypeRefund && common.UnmarshalJsonStr(l.Other, &ref) == nil && ref.Admin.ID > 0 {
				refs[ref.Admin.ID] = append(refs[ref.Admin.ID], l)
			}
		}
		cursor = batch[len(batch)-1].ID
	}
	plan := &holdCorrectionPlan{Version: 1, CapturedAt: time.Now().Unix(), MaxLogID: maxID, Candidates: []holdCorrection{}, Unresolved: []holdGap{}}
	for _, task := range tasks {
		async := task.PrivateData.AsyncBilling
		if async == nil || async.State != model.TaskBillingStateSettled || async.TargetQuota == nil || *async.TargetQuota != task.Quota {
			continue
		}
		scoped := []holdLog{}
		seen := map[int]bool{}
		conflict := owners[task.TaskID] != 1 || unreadable[fmt.Sprintf("%d:%d:%d:%s", task.UserId, task.PrivateData.TokenId, task.ChannelId, task.Properties.OriginModelName)]
		for _, l := range logs[task.TaskID] {
			if l.UserID != task.UserId || l.TokenID != task.PrivateData.TokenId || l.ChannelID != task.ChannelId || l.ModelName != task.Properties.OriginModelName {
				conflict = true
				continue
			}
			scoped = append(scoped, l)
			seen[l.ID] = true
		}
		// Explicit manual-refund references are part of the same task lifecycle.
		initial := append([]holdLog(nil), scoped...)
		for _, l := range initial {
			for _, refund := range refs[l.ID] {
				var relation struct {
					TaskID string `json:"task_id"`
				}
				if common.UnmarshalJsonStr(refund.Other, &relation) != nil || relation.TaskID != "" && relation.TaskID != task.TaskID || refund.UserID != task.UserId || refund.TokenID != task.PrivateData.TokenId {
					conflict = true
					continue
				}
				if !seen[refund.ID] {
					scoped = append(scoped, refund)
					seen[refund.ID] = true
				}
			}
		}
		sort.Slice(scoped, func(i, j int) bool { return scoped[i].ID < scoped[j].ID })
		net := int64(0)
		zero := []holdLog{}
		var applied *holdLog
		for _, l := range scoped {
			if l.Quota < 0 || l.Quota > math.MaxInt32 {
				conflict = true
			}
			if l.Type == model.LogTypeRefund {
				net -= l.Quota
			} else {
				net += l.Quota
			}
			var other map[string]any
			_ = common.UnmarshalJsonStr(l.Other, &other)
			if l.Type == model.LogTypeConsume && other["actual_quota"] == nil && other["pre_consumed_quota"] == nil {
				if l.Quota == 0 {
					zero = append(zero, l)
				}
				if admin, ok := other["admin_info"].(map[string]any); ok && admin["held_projection_correction"] != nil {
					copy := l
					applied = &copy
				}
			}
		}
		gap := int64(task.Quota) - net
		if gap <= 0 && applied == nil {
			continue
		}
		matching := []model.TaskCreateAttempt{}
		for _, a := range byTask[task.TaskID] {
			if a.UserID == task.UserId && a.TokenID == task.PrivateData.TokenId && a.AppID == task.AppID && a.ChannelID == task.ChannelId && a.PublicModel == task.Properties.OriginModelName {
				matching = append(matching, a)
			} else {
				conflict = true
			}
		}
		reason := "no_unique_transferred_wallet_hold"
		if len(matching) == 1 && matching[0].Status == model.TaskCreateAttemptComplete && matching[0].BillingHoldState == model.TaskCreateAttemptBillingTransferred && matching[0].BillingSource == "wallet" && matching[0].HeldQuota > 0 && matching[0].HeldQuota <= math.MaxInt32 {
			held := int64(matching[0].HeldQuota)
			reason = "initial_or_settlement_evidence_incomplete"
			target := holdLog{}
			if len(zero) == 1 {
				target = zero[0]
			}
			if applied != nil && len(zero) == 0 && gap == 0 && applied.Quota == held {
				target = *applied
			}
			// Exactly one initial record, plus one explicit terminal settlement.
			initials, finals := 0, 0
			normalized := append([]holdLog(nil), scoped...)
			for i, l := range normalized {
				var o map[string]any
				_ = common.UnmarshalJsonStr(l.Other, &o)
				if l.Type == model.LogTypeConsume && o["actual_quota"] == nil && o["pre_consumed_quota"] == nil {
					initials++
				}
				var amounts struct {
					Actual *int64 `json:"actual_quota"`
					Pre    *int64 `json:"pre_consumed_quota"`
				}
				if common.UnmarshalJsonStr(l.Other, &amounts) == nil && amounts.Actual != nil && amounts.Pre != nil && *amounts.Actual == int64(task.Quota) && *amounts.Pre == held {
					finals++
				}
				strip := l.ID == target.ID && applied != nil
				if strip {
					normalized[i].Quota = 0
				}
				raw, err := canonicalHoldMetadata(l.Other, strip)
				if err != nil {
					return nil, err
				}
				normalized[i].Other = string(raw)
			}
			switch {
			case initials != 1:
				reason = "initial_record_count_not_one"
			case target.ID == 0:
				reason = "no_unique_zero_initial_record"
			case finals != 1:
				reason = "settlement_hold_or_target_not_proven"
			case gap != held && applied == nil:
				reason = "net_gap_differs_from_transferred_hold"
			}

			if !conflict && target.ID > 0 && initials == 1 && finals == 1 && (gap == held || applied != nil && gap == 0) {
				attemptFacts := []any{}
				for _, a := range matching {
					attemptFacts = append(attemptFacts, []any{a.ID, a.PublicTaskID, a.UserID, a.TokenID, a.AppID, a.ChannelID, a.PublicModel, a.Status, a.BillingHoldState, a.BillingSource, a.HeldQuota})
				}
				evidence, err := common.Marshal([]any{task.ID, task.TaskID, task.UserId, task.AppID, task.ChannelId, task.Quota, task.Status, task.BillingState, task.Properties, task.PrivateData, attemptFacts, normalized})
				if err != nil {
					return nil, err
				}
				digest := fmt.Sprintf("%x", sha256.Sum256(evidence))
				if applied != nil {
					var o struct {
						Admin struct {
							Correction struct {
								Evidence string `json:"evidence_sha256"`
							} `json:"held_projection_correction"`
						} `json:"admin_info"`
					}
					if common.UnmarshalJsonStr(applied.Other, &o) != nil || o.Admin.Correction.Evidence != digest {
						return nil, fmt.Errorf("log %d correction evidence changed", applied.ID)
					}
				}
				loc := time.FixedZone("Asia/Shanghai", 8*3600)
				ts := time.Unix(target.CreatedAt, 0).In(loc)
				plan.Candidates = append(plan.Candidates, holdCorrection{TaskID: task.ID, UserID: task.UserId, LogID: target.ID, Period: time.Date(ts.Year(), ts.Month(), 1, 0, 0, 0, 0, loc).Unix(), Quota: held, Evidence: digest, Applied: applied != nil})
				continue
			}
		}
		if conflict {
			reason = "identity_conflict"
		}
		if gap > 0 {
			plan.Unresolved = append(plan.Unresolved, holdGap{TaskID: task.ID, UserID: task.UserId, Gap: gap, Reason: reason})
		}
	}
	return plan, nil
}

func applyHeldTaskProjection(tx *gorm.DB, approved *holdCorrectionPlan, generation int64, actor int, reason string) (int, error) {
	if err := model.RequireBillingStatementMaintenanceTx(tx, generation); err != nil {
		return 0, err
	}
	if approved.Version != 1 || len(approved.Candidates) == 0 || actor <= 0 || reason == "" {
		return 0, fmt.Errorf("an approved nonempty plan, operator and evidence are required")
	}
	current, err := auditHeldTaskProjection(tx)
	if err != nil {
		return 0, err
	}
	byID := map[int64]holdCorrection{}
	for _, c := range current.Candidates {
		byID[c.TaskID] = c
	}
	seen := map[int64]bool{}
	updated := 0
	for _, wanted := range approved.Candidates {
		actual, ok := byID[wanted.TaskID]
		if !ok || seen[wanted.TaskID] || wanted.Applied || actual.LogID != wanted.LogID || actual.UserID != wanted.UserID || actual.Period != wanted.Period || actual.Quota != wanted.Quota || actual.Evidence != wanted.Evidence {
			return 0, fmt.Errorf("task %d no longer matches approved evidence", wanted.TaskID)
		}
		seen[wanted.TaskID] = true
		if actual.Applied {
			continue
		}
		var log model.Log
		if err := tx.Select("id,quota,other").First(&log, wanted.LogID).Error; err != nil {
			return 0, err
		}
		var other map[string]json.RawMessage
		if common.UnmarshalJsonStr(log.Other, &other) != nil {
			return 0, fmt.Errorf("invalid log metadata")
		}
		admin := map[string]json.RawMessage{}
		if len(other["admin_info"]) > 0 {
			if err := common.Unmarshal(other["admin_info"], &admin); err != nil {
				return 0, err
			}
		}
		if admin == nil {
			admin = map[string]json.RawMessage{}
		}
		marker, err := common.Marshal(map[string]any{"version": 1, "evidence_sha256": wanted.Evidence, "operator_id": actor, "reason": reason, "at": time.Now().Unix(), "before_quota": 0, "after_quota": wanted.Quota})
		if err != nil {
			return 0, err
		}
		admin["held_projection_correction"] = marker
		other["admin_info"], err = common.Marshal(admin)
		if err != nil {
			return 0, err
		}
		encoded, err := common.Marshal(other)
		if err != nil {
			return 0, err
		}
		result := tx.Model(&model.Log{}).Where("id=? AND quota=0 AND other=?", log.Id, log.Other).Updates(map[string]any{"quota": wanted.Quota, "other": string(encoded)})
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected != 1 {
			return 0, fmt.Errorf("log changed during correction")
		}
		updated++
	}
	return updated, nil
}

// Canonicalize only the edited object; preserve numeric JSON values exactly.
func canonicalHoldMetadata(raw string, stripCorrection bool) ([]byte, error) {
	var other map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(raw, &other); err != nil {
		return nil, err
	}
	if value := other["admin_info"]; len(value) > 0 {
		var admin map[string]json.RawMessage
		if err := common.Unmarshal(value, &admin); err != nil {
			return nil, err
		}
		if stripCorrection {
			delete(admin, "held_projection_correction")
		}
		if len(admin) == 0 {
			delete(other, "admin_info")
		} else {
			encoded, err := common.Marshal(admin)
			if err != nil {
				return nil, err
			}
			other["admin_info"] = encoded
		}
	}
	return common.Marshal(other)
}
