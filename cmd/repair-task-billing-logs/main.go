// repair-task-billing-logs repairs a bounded SQLite log projection from frozen
// task facts. It never calls funding, settlement, refund or usage-counter APIs.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type scope struct {
	UserID, TokenID          int
	Model                    string
	Start, End, CreateTaskID int64
	FinalTaskID              int64
}

type repairReport struct {
	Tasks, MetadataUpdates, Creates int
	FinalLogs                       int
	NetBefore, NetAfter             int64
	Before, After                   taskLogIntegrity
}

func main() {
	var s scope
	dsn := flag.String("dsn", "", "local SQLite database (main and logs together)")
	apply := flag.Bool("apply", false, "apply verified log-only changes")
	backup := flag.String("backup", "", "new backup path, required with -apply")
	flag.IntVar(&s.UserID, "user-id", 0, "required customer ID")
	flag.IntVar(&s.TokenID, "token-id", 0, "required API key ID")
	flag.StringVar(&s.Model, "model", "", "required customer model")
	flag.Int64Var(&s.Start, "start", 0, "inclusive Unix timestamp")
	flag.Int64Var(&s.End, "end", 0, "inclusive Unix timestamp")
	flag.Int64Var(&s.CreateTaskID, "repair-create-task-id", 0, "explicit internal task row ID with missing initial log")
	flag.Int64Var(&s.FinalTaskID, "repair-final-task-id", 0, "explicit internal task row ID with missing completion log")
	flag.Parse()
	if *dsn == "" || s.UserID <= 0 || s.TokenID <= 0 || s.Model == "" || s.Start <= 0 || s.End < s.Start || s.End-s.Start > 31*86400 {
		fmt.Fprintln(os.Stderr, "explicit database, user, key, model and a period of at most 31 days are required")
		os.Exit(2)
	}
	mode := "ro"
	transactionOptions := ""
	if *apply {
		mode = "rw"
		// Reserve the SQLite writer before reading the repair plan. A deferred
		// read transaction cannot upgrade after a live worker commits in WAL mode.
		transactionOptions = "&_txlock=immediate&_pragma=busy_timeout(5000)"
	}
	db, err := gorm.Open(sqlite.Open(*dsn+"?mode="+mode+transactionOptions), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		fail("open database")
	}
	if *apply {
		if *backup == "" {
			fail("-backup is required")
		}
		f, err := os.OpenFile(*backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fail("backup path must be new and writable")
		}
		_ = f.Close()
		if db.Exec("VACUUM INTO ?", *backup).Error != nil {
			fail("create consistent SQLite backup")
		}
	}
	r, err := repair(db, s, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("applied=%t tasks=%d metadata_updates=%d initial_logs_added=%d final_logs_added=%d net_quota_before=%d net_quota_after=%d\n", *apply, r.Tasks, r.MetadataUpdates, r.Creates, r.FinalLogs, r.NetBefore, r.NetAfter)
	fmt.Printf("integrity_before=%s integrity_after=%s settled_quota=%d net_difference_before=%d net_difference_after=%d missing_initial_logs=%d unlinked_initial_logs=%d unreconciled_tasks=%d usage_mismatches=%d missing_final_logs=%d\n", r.Before.Status, r.After.Status, r.After.SettledQuota, r.Before.NetDifference, r.After.NetDifference, r.After.MissingInitialLogs, r.After.UnlinkedInitialLogs, r.After.UnreconciledTasks, r.After.UsageMismatches, r.After.MissingFinalLogs)
	if r.After.Status == "mismatch" || r.After.Status == "incomplete" {
		os.Exit(1)
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func repair(db *gorm.DB, s scope, apply bool) (repairReport, error) {
	var report repairReport
	err := db.Transaction(func(tx *gorm.DB) error {
		var tasks []model.Task
		if tx.Select("id", "task_id", "user_id", "app_id", "channel_id", "group", "platform", "quota", "status", "billing_state", "submit_time", "finish_time", "properties", "private_data").
			Where("user_id = ? AND submit_time >= ? AND submit_time <= ?", s.UserID, s.Start, s.End).Find(&tasks).Error != nil {
			return fmt.Errorf("cannot read task facts")
		}
		byID := map[string]*model.Task{}
		var createTask *model.Task
		for i := range tasks {
			t := &tasks[i]
			if t.Properties.OriginModelName != s.Model || t.PrivateData.TokenId != s.TokenID {
				continue
			}
			if t.PrivateData.AsyncBilling == nil || t.PrivateData.AsyncBilling.TieredSnapshot == nil {
				return fmt.Errorf("task %d lacks frozen expression", t.ID)
			}
			if t.PrivateData.BillingContext != nil && t.PrivateData.BillingContext.ContractFact != nil {
				return fmt.Errorf("task %d has a contract requiring a separately reviewed repair", t.ID)
			}
			byID[t.TaskID] = t
			report.Tasks++
			if t.ID == s.CreateTaskID {
				createTask = t
			}
		}
		if s.CreateTaskID != 0 && createTask == nil {
			return fmt.Errorf("requested task is outside the repair scope")
		}
		// Read the complete scoped log history so a delayed or cross-period
		// initial entry cannot be mistaken for a missing charge.
		var logs []model.Log
		if tx.Select("id", "user_id", "token_id", "channel_id", "model_name", "type", "quota", "created_at", "other", "completion_tokens", "prompt_tokens", "username", "token_name", "group").
			Where("user_id = ? AND token_id = ? AND model_name = ? AND type IN ?", s.UserID, s.TokenID, s.Model, []int{model.LogTypeConsume, model.LogTypeRefund}).Find(&logs).Error != nil {
			return fmt.Errorf("cannot read scoped logs")
		}
		report.Before = inspectTaskLogIntegrity(byID, logs, s)
		createExists := false
		createNet := int64(0)
		unlinkedCreate := false
		for logIndex, log := range logs {
			var other map[string]any
			if common.UnmarshalJsonStr(log.Other, &other) != nil {
				return fmt.Errorf("invalid metadata at log %d", log.Id)
			}
			taskID, _ := other["task_id"].(string)
			isCreate := isInitialTaskLog(log, other)
			if createTask != nil && taskID == "" && isCreate && log.ChannelId == createTask.ChannelId {
				unlinkedCreate = true
			}
			if log.CreatedAt < s.Start || log.CreatedAt > s.End {
				if createTask != nil && taskID == createTask.TaskID {
					return fmt.Errorf("task has logs outside the selected period; select its complete lifecycle")
				}
				continue
			}
			signed := int64(log.Quota)
			if log.Type == model.LogTypeRefund {
				signed = -signed
			}
			report.NetBefore += signed
			t := byID[taskID]
			if t == nil {
				continue
			}
			if log.ChannelId != t.ChannelId {
				return fmt.Errorf("channel conflict at log %d", log.Id)
			}
			if createTask != nil && t.ID == createTask.ID {
				createNet += signed
				if isCreate {
					if createExists {
						return fmt.Errorf("duplicate initial logs")
					}
					createExists = true
				}
			}
			snap := t.PrivateData.AsyncBilling.TieredSnapshot
			mode := model.BillingStatementExpressionMode(snap.ExprString, snap.UsageUnits)
			if mode != model.BillingReconciliationModeToken {
				return fmt.Errorf("task %d is not verified token expression billing", t.ID)
			}
			expr := base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
			if existing, ok := other["expr_b64"].(string); ok && existing != expr {
				return fmt.Errorf("expression conflict at log %d", log.Id)
			}
			if ratio, ok := other["group_ratio"].(float64); !ok || ratio != snap.GroupRatio {
				return fmt.Errorf("discount conflict at log %d", log.Id)
			}
			admin, _ := other["admin_info"].(map[string]any)
			if admin == nil {
				admin = map[string]any{}
				other["admin_info"] = admin
			}
			statement, _ := admin["statement_snapshot"].(map[string]any)
			if statement == nil {
				statement = map[string]any{}
				admin["statement_snapshot"] = statement
			}
			before, err := common.Marshal(other)
			if err != nil {
				return fmt.Errorf("cannot encode log %d", log.Id)
			}
			event := "adjustment"
			if isCreate {
				event = "create"
			} else if log.Type == model.LogTypeRefund && other["actual_quota"] == nil && other["pre_consumed_quota"] == nil {
				event = "refund"
			}
			other["task_billing_event"] = event
			other["billing_mode"] = "tiered_expr"
			other["expr_b64"] = expr
			statement["billing_mode"] = mode
			statement["expr_b64"] = expr
			statement["group_ratio"] = snap.GroupRatio
			if len(snap.UsageUnits) > 0 {
				other["usage_units"] = snap.UsageUnits
				statement["usage_units"] = snap.UsageUnits
			}
			completion := log.CompletionTokens
			async := t.PrivateData.AsyncBilling
			if event == "adjustment" && async.State == model.TaskBillingStateSettled {
				computed, _, computeErr := service.ComputeTaskTieredBilling(t)
				if computeErr == nil && computed.Clamp == nil && computed.ActualQuotaAfterGroup == t.Quota {
					completion = async.ActualTokens
					projected, projectionErr := service.BuildTaskBillingDeliveryLog(t, model.TaskBillingDelivery{Event: "adjustment", CompletionTokens: completion, UsageReported: async.ActualUsageReported})
					if projectionErr != nil {
						return projectionErr
					}
					var facts map[string]any
					if err := common.UnmarshalJsonStr(projected.Other, &facts); err != nil {
						return err
					}
					for _, key := range []string{"matched_tier", "request_rules", "usage_facts"} {
						if value, ok := facts[key]; ok {
							other[key] = value
						}
					}
				}
			}
			after, err := common.Marshal(other)
			if err != nil {
				return fmt.Errorf("cannot encode corrected log")
			}
			if reflect.DeepEqual(before, after) && completion == log.CompletionTokens {
				continue
			}
			admin["billing_statement_repair"] = map[string]any{"version": 1, "task_row_id": t.ID, "at": time.Now().Unix(), "reason": "frozen_task_billing_facts"}
			after, err = common.Marshal(other)
			if err != nil {
				return fmt.Errorf("cannot encode repair audit")
			}
			report.MetadataUpdates++
			if apply {
				result := tx.Model(&model.Log{}).Where("id = ? AND other = ?", log.Id, log.Other).Updates(map[string]any{"other": string(after), "completion_tokens": completion})
				if result.Error != nil {
					return fmt.Errorf("cannot write log %d; repair rolled back", log.Id)
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("log %d changed concurrently; repair rolled back", log.Id)
				}
			}
			logs[logIndex].Other, logs[logIndex].CompletionTokens = string(after), completion
		}
		report.NetAfter = report.NetBefore
		var err error
		logs, err = repairMissingFinalLog(tx, byID, logs, s, apply, &report)
		if err != nil {
			return err
		}
		if createTask != nil && report.FinalLogs > 0 && createTask.ID == s.FinalTaskID {
			createNet = int64(createTask.Quota)
		}
		if createTask == nil {
			return finalizeRepairIntegrity(&report, byID, logs, s, apply)
		}
		if createExists {
			if createNet != int64(createTask.Quota) {
				return fmt.Errorf("existing task logs do not reconcile to settlement")
			}
			return finalizeRepairIntegrity(&report, byID, logs, s, apply)
		}
		if unlinkedCreate {
			return fmt.Errorf("an unlinked initial log may belong to the requested task")
		}
		async := createTask.PrivateData.AsyncBilling
		if async.State != model.TaskBillingStateSettled || async.TargetQuota == nil || *async.TargetQuota != createTask.Quota {
			return fmt.Errorf("task settlement is not complete")
		}
		var attempts []model.TaskCreateAttempt
		if tx.Select("id", "held_quota", "status", "billing_hold_state").Where("public_task_id = ? AND user_id = ? AND token_id = ? AND app_id = ? AND channel_id = ? AND public_model = ?", createTask.TaskID, s.UserID, s.TokenID, createTask.AppID, createTask.ChannelId, s.Model).Find(&attempts).Error != nil || len(attempts) != 1 {
			return fmt.Errorf("a unique matching creation attempt is required")
		}
		a := attempts[0]
		if a.Status != model.TaskCreateAttemptComplete || a.BillingHoldState != model.TaskCreateAttemptBillingTransferred || a.HeldQuota <= 0 {
			return fmt.Errorf("creation hold was not durably transferred")
		}
		if createNet != int64(createTask.Quota)-int64(a.HeldQuota) {
			return fmt.Errorf("settlement logs do not reconcile to the transferred hold")
		}
		if async.BillingProbe == nil {
			return fmt.Errorf("missing frozen billing probe")
		}
		result, _, err := service.ComputeTaskTieredBilling(createTask)
		if err != nil || result.Clamp != nil || result.ActualQuotaAfterGroup != createTask.Quota {
			return fmt.Errorf("frozen usage does not reproduce settled quota")
		}
		var user struct{ Username string }
		var token struct{ Name string }
		if tx.Model(&model.User{}).Select("username").Where("id = ?", s.UserID).Take(&user).Error != nil || tx.Table("tokens").Select("name").Where("id = ?", s.TokenID).Take(&token).Error != nil {
			return fmt.Errorf("missing customer or key identity")
		}
		snap := async.TieredSnapshot
		expr := base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
		other := model.NewLogOther()
		other.SetPublic("is_task", true)
		other.SetPublic("task_id", createTask.TaskID)
		other.SetPublic("task_billing_event", "create")
		other.SetPublic("billing_mode", "tiered_expr")
		other.SetPublic("expr_b64", expr)
		if len(snap.UsageUnits) > 0 {
			other.SetPublic("usage_units", snap.UsageUnits)
		}
		other.SetPublic("model_price", 0)
		other.SetPublic("group_ratio", snap.GroupRatio)
		other.SetAdmin("statement_snapshot", map[string]any{"snapshot_version": 1, "billing_mode": "token", "group_ratio": snap.GroupRatio, "expr_b64": expr, "provider_model": createTask.Properties.UpstreamModelName, "usage_units": snap.UsageUnits})
		other.SetAdmin("billing_statement_repair", map[string]any{"version": 1, "task_row_id": createTask.ID, "attempt_row_id": a.ID, "at": time.Now().Unix(), "reason": "missing_initial_log_from_transferred_hold"})
		log := model.Log{UserId: s.UserID, Username: user.Username, TokenId: s.TokenID, TokenName: token.Name, ModelName: s.Model, ChannelId: createTask.ChannelId, Group: createTask.Group, CreatedAt: createTask.SubmitTime, Type: model.LogTypeConsume, Quota: a.HeldQuota, Other: other.JSONString(), Content: "Initial task hold log restored from durable facts", RequestId: fmt.Sprintf("billing-log-repair:%d:create", a.ID)}
		if apply && tx.Create(&log).Error != nil {
			return fmt.Errorf("cannot insert initial log; repair rolled back")
		}
		report.Creates = 1
		report.NetAfter += int64(a.HeldQuota)
		return finalizeRepairIntegrity(&report, byID, append(logs, log), s, apply)
	})
	return report, err
}
