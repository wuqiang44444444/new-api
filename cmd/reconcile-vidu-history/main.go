// reconcile-vidu-history is offline maintenance for Vidu tasks submitted under
// the official 1.3.5 parser before Vidu had its own protocol. It never creates
// provider tasks, changes frozen contracts, or writes balances directly.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type evidence struct {
	Tokens                        int
	ResultURL, Resolution, Digest string
	Duration                      int
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("reconcile-vidu-history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", "", "existing local SQLite database (main and logs together)")
	taskID := flags.String("task-id", "", "one public task ID")
	operator := flags.Int("operator-id", 0, "authorized Root operator for audit attribution")
	expected := flags.Int("expected-hold", 0, "previously verified held quota")
	ref := flags.String("reference", "", "reconciliation reference, no provider payloads")
	backup := flags.String("backup", "", "new consistent backup path required for apply")
	apply := flags.Bool("apply", false, "apply verified observation and frozen-price settlement")
	offline := flags.Bool("offline-no-redis", false, "operator confirms all application workers stopped and no Redis")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(output)
			flags.PrintDefaults()
			return nil
		}
		return errors.New("invalid arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *database == "" || *taskID == "" || *operator <= 0 || *expected <= 0 || *expected > math.MaxInt32 || len(*ref) == 0 || len(*ref) > 128 || strings.ContainsAny(*ref, "\r\n/?=&") || (*apply && (!*offline || *backup == "")) {
		return errors.New("invalid scope or missing offline backup confirmation")
	}
	path, err := filepath.Abs(*database)
	if err != nil {
		return errors.New("invalid database path")
	}
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return errors.New("existing SQLite file required")
	}
	if os.Getenv("REDIS_CONN_STRING") != "" || common.RDB != nil || common.MemoryCacheEnabled || common.BatchUpdateEnabled {
		return errors.New("offline environment required")
	}
	uri := url.URL{Scheme: "file", Path: path}
	q := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(5000)"}}
	if *apply {
		q.Set("mode", "rw")
		q.Set("_txlock", "immediate")
	}
	uri.RawQuery = q.Encode()
	db, err := gorm.Open(sqlite.Open(uri.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return errors.New("database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("database unavailable")
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousLogConsume := common.RedisEnabled, common.LogConsumeEnabled
	model.DB, model.LOG_DB = db, db
	defer func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.LogConsumeEnabled = previousRedis, previousLogConsume
	}()
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	var task model.Task
	if db.Where("task_id = ?", *taskID).First(&task).Error != nil {
		return errors.New("task not found")
	}
	var root model.User
	if db.First(&root, *operator).Error != nil || root.Role != common.RoleRootUser || root.Status != common.UserStatusEnabled {
		return errors.New("enabled Root operator required")
	}
	if err = validateHistoricalTask(&task, *expected); err != nil {
		return err
	}
	eventID := fmt.Sprintf("vidu-history-success:%d", task.ID)
	var prior []model.AuditLog
	if db.Where("event_id = ?", eventID).Find(&prior).Error != nil {
		return errors.New("audit unavailable")
	}
	if len(prior) > 0 {
		// Durable audited evidence owns recovery. Provider retention and availability
		// must not gate an already accepted settlement or log delivery.
		if len(prior) != 1 || !prior[0].Success || prior[0].Action != "task.vidu_verified_success" || task.Status != model.TaskStatusSuccess || !task.PrivateData.AsyncBilling.ActualUsageReported {
			return errors.New("previous reconciliation conflicts")
		}
		var recorded struct {
			Reference string `json:"reference"`
			TaskID    string `json:"task_id"`
			Tokens    int    `json:"completion_tokens"`
			Held      int    `json:"held_quota"`
			Target    int    `json:"target_quota"`
		}
		encoded, encodeErr := common.Marshal(prior[0].Other.RootInfo)
		if encodeErr != nil || common.Unmarshal(encoded, &recorded) != nil || recorded.Reference != *ref || recorded.TaskID != task.TaskID || recorded.Tokens != task.PrivateData.AsyncBilling.ActualTokens || recorded.Held != *expected || recorded.Target < 0 || task.PrivateData.AsyncBilling.TargetQuota == nil || *task.PrivateData.AsyncBilling.TargetQuota != recorded.Target || (task.PrivateData.AsyncBilling.State == model.TaskBillingStateSettled && task.Quota != recorded.Target) {
			return errors.New("reconciliation reference conflicts")
		}
		if *apply {
			if err := createRecoveryBackup(db, *backup); err != nil {
				return err
			}
			return finishFunding(&task, output)
		}
		fmt.Fprintf(output, "task=%s already_recorded=true quota=%d billing=%s\n", task.TaskID, task.Quota, task.BillingState)
		return nil
	}
	if task.Status != model.TaskStatusReconciliationRequired || task.PrivateData.AsyncBilling.ActualUsageReported || task.PrivateData.AsyncBilling.TargetQuota != nil || task.PrivateData.AsyncBilling.State != model.TaskBillingStatePending || task.Quota != *expected {
		return errors.New("task or held funds changed; recheck")
	}
	observed, err := queryEvidence(&task)
	if err != nil {
		return err
	}
	frozenBytes, err := common.Marshal(task.PrivateData)
	if err != nil {
		return errors.New("invalid frozen task")
	}
	a := task.PrivateData.AsyncBilling
	a.ActualTokens = observed.Tokens
	a.ActualUsageReported = true
	a.ActualUsageSource = "operator_verified_provider_query"
	a.ActualUsageEvidence = map[string]int{"usage.completion_tokens": observed.Tokens}
	result, _, err := service.ComputeTaskTieredBilling(&task)
	if err != nil || result.Clamp != nil || result.ActualQuotaAfterGroup < 0 {
		return errors.New("frozen price cannot be verified")
	}
	target := result.ActualQuotaAfterGroup
	fmt.Fprintf(output, "task=%s verified_success=true tokens=%d held_quota=%d target_quota=%d return_quota=%d apply=%t\n", task.TaskID, observed.Tokens, *expected, target, *expected-target, *apply)
	if !*apply {
		return nil
	}
	if err := createRecoveryBackup(db, *backup); err != nil {
		return err
	}
	task.Status = model.TaskStatusSuccess
	task.Progress = "100%"
	task.FailReason = ""
	task.FinishTime = common.GetTimestamp()
	// Only the normal protected result pointer is saved; the original response,
	// headers and credentials never enter the audit or diagnostic output.
	task.PrivateData.ResultURL = observed.ResultURL
	a.Operation = "settle"
	a.Reason = "Verified historical Vidu usage; frozen price reconciliation"
	a.TargetQuota = &target
	a.CalculationVersion = 1
	a.CalculationSource = "tiered_expr"
	a.Calculation = result.Calculation
	task.Data, err = common.Marshal(map[string]any{"status": "succeeded", "duration": observed.Duration, "resolution": observed.Resolution, "usage": map[string]int{"completion_tokens": observed.Tokens, "total_tokens": observed.Tokens}})
	if err != nil {
		return errors.New("normalized observation failed")
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var current model.Task
		if tx.First(&current, task.ID).Error != nil {
			return errors.New("task disappeared")
		}
		currentBytes, _ := common.Marshal(current.PrivateData)
		if current.Status != model.TaskStatusReconciliationRequired || current.Quota != *expected || string(currentBytes) != string(frozenBytes) || current.VideoRefundState != "" {
			return errors.New("concurrent task change; rolled back")
		}
		updates := map[string]any{"status": task.Status, "progress": task.Progress, "finish_time": task.FinishTime, "fail_reason": "", "data": task.Data, "private_data": task.PrivateData, "usage_review_operator_id": *operator, "usage_review_reference": *ref}
		if tx.Model(&current).Updates(updates).Error != nil {
			return errors.New("observation write failed")
		}
		audit := model.AuditLog{EventId: eventID, UserId: root.Id, Username: root.Username, ActorRole: root.Role, CreatedAt: common.GetTimestamp(), Category: model.AuditCategoryOperation, Action: "task.vidu_verified_success", AuthMethod: "offline_maintenance", Success: true, Content: "Verified historical Vidu success and usage; settle using frozen price", Other: model.AuditOther{RootInfo: model.AuditFields{"reference": *ref, "response_sha256": observed.Digest, "task_id": task.TaskID, "completion_tokens": observed.Tokens, "held_quota": *expected, "target_quota": target}}}
		if tx.Create(&audit).Error != nil {
			return errors.New("audit write failed")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return finishFunding(&task, output)
}
func createRecoveryBackup(db *gorm.DB, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("new backup path required")
	}
	if file.Close() != nil || db.Exec("VACUUM INTO ?", path).Error != nil {
		return errors.New("consistent backup failed; nothing changed")
	}
	return nil
}
func validateHistoricalTask(t *model.Task, hold int) error {
	p := t.PrivateData
	a := p.AsyncBilling
	if t.UserId <= 0 || t.AppID <= 0 || p.TokenId != t.AppID || t.VideoRefundState != "" || p.BillingSource != "wallet" || a == nil || a.TieredSnapshot == nil || p.Execution == nil || p.Execution.TaskPlugin == nil || p.Execution.TaskPlugin.Key != "seedance-link" || p.Execution.TaskPlugin.Version != "1.3.5" || p.UpstreamTaskID == "" {
		return errors.New("outside historical Vidu recovery contract")
	}
	cn := p.VideoUpstreamProtocol == dto.VideoUpstreamProtocolModelArkV3Volcengine && p.VideoUpstreamQueryBaseURL == "https://api.vidu.cn/ent" && t.Properties.UpstreamModelName == "viduq3-drama-std"
	global := p.VideoUpstreamProtocol == dto.VideoUpstreamProtocolModelArkV3BytePlus && p.VideoUpstreamQueryBaseURL == "https://api.vidu.com/ent" && t.Properties.UpstreamModelName == "viduq3-drama-ab-std"
	if !cn && !global {
		return errors.New("frozen connection or model is not the reviewed Vidu history")
	}
	if a.State != model.TaskBillingStateSettled && t.Quota != hold {
		return errors.New("held quota changed")
	}
	return nil
}
func queryEvidence(t *model.Task) (evidence, error) {
	var out evidence
	p := t.PrivateData
	req, err := http.NewRequest(http.MethodGet, p.VideoUpstreamQueryBaseURL+"/api/v3/contents/generations/tasks/"+url.PathEscape(p.UpstreamTaskID), nil)
	if err != nil {
		return out, errors.New("query construction failed")
	}
	req.Header.Set("Authorization", "Bearer "+p.Key)
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}
	resp, err := client.Do(req)
	if err != nil {
		return out, errors.New("provider query unavailable")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || resp.StatusCode != 200 {
		return out, errors.New("provider query unverified")
	}
	var body struct {
		ID, Status, Model string
		Content           struct {
			VideoURL string `json:"video_url"`
		}
		Usage struct {
			Completion json.RawMessage `json:"completion_tokens"`
			Total      json.RawMessage `json:"total_tokens"`
		}
		Duration   json.RawMessage
		Resolution string
		Error      json.RawMessage
	}
	if common.Unmarshal(raw, &body) != nil || body.ID != p.UpstreamTaskID || body.Status != "succeeded" || body.Model != t.Properties.UpstreamModelName || body.Content.VideoURL == "" || (len(body.Error) > 0 && string(body.Error) != "null") {
		return out, errors.New("provider identity, success or result unverified")
	}
	parse := func(raw json.RawMessage) (int, error) {
		s := string(raw)
		if len(s) > 0 && s[0] == '"' {
			if common.Unmarshal(raw, &s) != nil {
				return 0, errors.New("invalid integer")
			}
		}
		if s == "" || (len(s) > 1 && s[0] == '0') {
			return 0, errors.New("invalid integer")
		}
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				return 0, errors.New("invalid integer")
			}
		}
		n, e := strconv.ParseInt(s, 10, 32)
		return int(n), e
	}
	out.Tokens, err = parse(body.Usage.Completion)
	if err != nil {
		return out, errors.New("completion usage unverified")
	}
	total, err := parse(body.Usage.Total)
	if err != nil || total < out.Tokens {
		return out, errors.New("total usage unverified")
	}
	out.Duration, err = parse(body.Duration)
	if err != nil || out.Duration < 1 || out.Duration > 15 {
		return out, errors.New("duration unverified")
	}
	resultURL, err := url.Parse(body.Content.VideoURL)
	if err != nil || resultURL.Scheme != "https" || resultURL.User != nil || resultURL.Host == "" {
		return out, errors.New("result URL unverified")
	}
	out.ResultURL = body.Content.VideoURL
	out.Resolution = body.Resolution
	out.Digest = fmt.Sprintf("%x", sha256.Sum256(raw))
	return out, nil
}
func finishFunding(task *model.Task, output io.Writer) error {
	a := task.PrivateData.AsyncBilling
	if a == nil || a.TargetQuota == nil {
		return errors.New("durable settlement target missing")
	}
	if _, _, err := model.ApplyTaskBillingTarget(task, *a.TargetQuota); err != nil {
		return errors.New("observation audited; funding pending, repeat the same scope")
	}
	var events []model.TaskBillingDelivery
	if model.DB.Where("task_row_id = ? AND delivered_at = 0", task.ID).Order("id").Find(&events).Error != nil {
		return errors.New("funding committed; log lookup pending")
	}
	for _, event := range events {
		if model.DeliverTaskBillingLog(context.Background(), event.ID, service.BuildTaskBillingDeliveryLog) != nil {
			return errors.New("funding committed; log delivery pending")
		}
	}
	var final model.Task
	if model.DB.First(&final, task.ID).Error != nil || final.BillingState != model.TaskBillingStateSettled || final.Quota != *a.TargetQuota {
		return errors.New("settlement verification failed")
	}
	fmt.Fprintf(output, "task=%s status=%s billing=%s quota=%d logs_delivered=true\n", final.TaskID, final.Status, final.BillingState, final.Quota)
	return nil
}
