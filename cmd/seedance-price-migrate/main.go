// seedance-price-migrate is the explicit offline/maintenance migration tool
// for Seedance price expressions (plan §7). It is read-only against the
// database: it reads a local SQLite snapshot, converts legacy c/_task
// expressions into u() dollar-expression candidates, proves per-model
// equivalence (model USD fee and the final integer quota must match exactly),
// and reports every model with a deterministic outcome. -emit only writes a
// change-batch JSON (per-model expected_version plus a restore list) that the
// maintenance window applies through the existing model pricing transaction;
// the tool never writes the database and never participates in online paths.
//
// Expressions that cannot be proven (unknown identifiers, dynamic paths,
// indirect function calls, undeclared fields such as size_multiplier)
// never enter the batch and are listed for manual handling.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// pricingOptionKeys mirrors model.modelPricingOptionKeys so the emitted
// expected_version matches the server's ModelPricingVersion computation.
var pricingOptionKeys = []string{
	"AudioCompletionRatio", "AudioRatio", "CacheRatio", "CompletionRatio",
	"CreateCacheRatio", "ImageRatio", "ModelPrice", "ModelRatio",
	"billing_setting.billing_expr", "billing_setting.billing_mode",
	"task_billing_setting.preconsume_tokens",
}

const groupRatioOptionKey = "group_ratio_setting.group_ratio"

const maxProofVectors = 4096

type migrationLine struct {
	Model      string   `json:"model"`
	Protocols  []string `json:"protocols"`
	Outcome    string   `json:"outcome"`
	Detail     string   `json:"detail,omitempty"`
	Candidate  string   `json:"candidate,omitempty"`
	VectorsRun int      `json:"vectors_run"`
}

type seedancePriceChange struct {
	ModelName       string         `json:"model_name"`
	ExpectedVersion string         `json:"expected_version"`
	Pricing         map[string]any `json:"pricing"`
}

type restoreEntry struct {
	ModelName string         `json:"model_name"`
	Pricing   map[string]any `json:"pricing"`
}

type emitDocument struct {
	GeneratedFrom string                `json:"generated_from"`
	Changes       []seedancePriceChange `json:"changes"`
	Restore       []restoreEntry        `json:"restore"`
}

func main() {
	dsn := flag.String("dsn", "", "SQLite snapshot path (read-only)")
	emit := flag.String("emit", "", "write the proven change batch JSON to this path (the database is never written)")
	models := flag.String("models", "", "restrict to these comma-separated customer models")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: seedance-price-migrate -dsn <sqlite file> [-emit changes.json] [-models a,b]")
		os.Exit(2)
	}
	db, err := gorm.Open(sqlite.Open(*dsn+"?mode=ro"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		fail("open snapshot", err)
	}

	options := readOptions(db)
	modeByModel := decodeMap(options["billing_setting.billing_mode"])
	exprByModel := decodeMap(options["billing_setting.billing_expr"])
	preconsumeByModel := decodeMap(options["task_billing_setting.preconsume_tokens"])

	protocolsByModel := readSeedanceProtocols(db)

	wanted := map[string]bool{}
	for _, model := range strings.Split(*models, ",") {
		if model = strings.TrimSpace(model); model != "" {
			wanted[model] = true
		}
	}
	for modelName := range wanted {
		if _, exists := protocolsByModel[modelName]; !exists {
			fail("select models", fmt.Errorf("requested model %q has no Seedance channel in this snapshot", modelName))
		}
	}

	groupRatios := decodeMap(options[groupRatioOptionKey])
	proofGroupRatios := []float64{0, 1}
	seenRatios := map[float64]bool{0: true, 1: true}
	for _, key := range sortedKeys(groupRatios) {
		value, ok := groupRatios[key].(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			fail("read group ratio", fmt.Errorf("invalid group multiplier"))
		}
		if !seenRatios[value] {
			proofGroupRatios = append(proofGroupRatios, value)
			seenRatios[value] = true
		}
	}

	changes := []seedancePriceChange{}
	restore := []restoreEntry{}
	lines := []migrationLine{}
	for _, model := range sortedKeys(protocolsByModel) {
		if len(wanted) > 0 && !wanted[model] {
			continue
		}
		protocolNames := sortedProtocolNames(protocolsByModel[model])
		line := migrationLine{Model: model, Protocols: protocolNames}
		modeValue, _ := modeByModel[model].(string)
		exprValue, _ := exprByModel[model].(string)
		budget, hasBudget := intValue(preconsumeByModel[model])
		if _, configured := preconsumeByModel[model]; configured && !hasBudget {
			line.Outcome = "blocked"
			line.Detail = "invalid customer token budget"
			lines = append(lines, line)
			continue
		}

		if modeValue != "tiered_expr" {
			line.Outcome = "blocked"
			line.Detail = "customer model mode is not tiered_expr"
			lines = append(lines, line)
			continue
		}
		if strings.TrimSpace(exprValue) == "" {
			line.Outcome = "blocked"
			line.Detail = "tiered_expr mode without a customer-key expression; runtime would reject new acceptance"
			lines = append(lines, line)
			continue
		}
		// 与运行时归属一致：启用优先；没有启用渠道时取停用协议交集。
		schemas := make([]map[string]jsplugin.UsageFieldSchema, 0, len(protocolsByModel[model]))
		for protocol := range protocolsByModel[model] {
			schemas = append(schemas, seedancebilling.UsageFieldsForProtocol(protocol))
		}
		schema := seedancebilling.IntersectUsageFields(schemas...)

		candidate, convertErr := convertLegacyExpression(exprValue, schema)
		if convertErr != nil {
			line.Outcome = "manual"
			line.Detail = convertErr.Error()
			lines = append(lines, line)
			continue
		}
		line.Candidate = candidate
		budgetRequired := false
		for protocol := range protocolsByModel[model] {
			if seedancebilling.RequiresTokenBudget(protocol, candidate) {
				budgetRequired = true
			}
		}
		if budgetRequired && !hasBudget {
			line.Outcome = "blocked"
			line.Detail = "required customer token budget is missing"
			lines = append(lines, line)
			continue
		}
		if err := seedancebilling.ValidateTaskExpression(candidate, schema); err != nil {
			line.Outcome = "manual"
			line.Detail = "candidate failed the field contract: " + err.Error()
			lines = append(lines, line)
			continue
		}

		vectors, vectorErr := proofVectors(candidate, schema, budget, hasBudget)
		if vectorErr != nil {
			line.Outcome = "manual"
			line.Detail = vectorErr.Error()
			lines = append(lines, line)
			continue
		}
		if detail := proveEquivalence(exprValue, candidate, vectors, proofGroupRatios); detail != "" {
			line.Outcome = "blocked"
			line.Detail = detail
			line.VectorsRun = len(vectors)
			lines = append(lines, line)
			continue
		}
		line.Outcome = "converted"
		line.VectorsRun = len(vectors)
		line.Detail = fmt.Sprintf("proved at group ratios %v (final integer quota identical)", proofGroupRatios)
		lines = append(lines, line)

		pricing := configuredValues(options, model)
		pricing["billing_setting.billing_mode"] = "tiered_expr"
		pricing["billing_setting.billing_expr"] = candidate
		if hasBudget {
			pricing["task_billing_setting.preconsume_tokens"] = budget
		}
		changes = append(changes, seedancePriceChange{
			ModelName:       model,
			ExpectedVersion: modelPricingVersion(configuredValues(options, model)),
			Pricing:         pricing,
		})
		restore = append(restore, restoreEntry{ModelName: model, Pricing: configuredValues(options, model)})
	}

	fmt.Println("== Seedance 价格表达式迁移预览 ==")
	fmt.Printf("snapshot_dsn: %s\n", *dsn)
	fmt.Println("note: 本工具只读；-emit 仅生成变更批次 JSON，应用必须走维护窗口内的模型价格事务。")
	fmt.Println("note: 输入必须是切换前的旧计价快照；常量表达式无法仅靠语法判断单位，禁止对已迁移配置再次转换。")
	fmt.Println("note: 证明向量覆盖枚举组合、秒数/布尔边界、显式零、预算与 int32 上界 token；保留完整金额与数值阈值，无法保持输入或规则追踪的表达式转人工。")
	fmt.Println()
	fmt.Println("model | protocols | outcome | vectors | candidate/detail")
	for _, line := range lines {
		detail := line.Candidate
		if line.Detail != "" {
			if detail != "" {
				detail += " | "
			}
			detail += line.Detail
		}
		fmt.Printf("%s | %s | %s | %d | %s\n", line.Model, strings.Join(line.Protocols, ","), line.Outcome, line.VectorsRun, valueOr(detail, "-"))
	}
	converted, blocked, manual := 0, 0, 0
	for _, line := range lines {
		switch line.Outcome {
		case "converted":
			converted++
		case "blocked":
			blocked++
		default:
			manual++
		}
	}
	fmt.Println()
	fmt.Printf("converted=%d blocked=%d manual_or_skipped=%d\n", converted, blocked, manual)
	fmt.Println("restore_list_models:", strings.Join(restoreModels(restore), ", "))

	if *emit != "" {
		if len(lines) == 0 {
			fail("emit", fmt.Errorf("no Seedance models in scope"))
		}
		if blocked > 0 || containsOutcome(lines, "manual") {
			fmt.Fprintln(os.Stderr, "emit refused: blocked or manual models remain in scope; resolve them or restrict -models to the proven batch")
			os.Exit(1)
		}
		document := emitDocument{GeneratedFrom: *dsn, Changes: changes, Restore: restore}
		encoded, err := common.Marshal(document)
		if err != nil {
			fail("marshal emit document", err)
		}
		if err := os.WriteFile(*emit, encoded, 0o600); err != nil {
			fail("write emit document", err)
		}
		fmt.Printf("wrote %d changes and %d restore entries to %s\n", len(changes), len(restore), *emit)
	}
}

type proofVector struct {
	ProbeBody []byte
	Tokens    int
}

// proofVectors enumerates frozen-condition combinations and meter values from
// the declared field contract: enums over their values, booleans at both
// poles, seconds at 5/3600, tokens at zero, one, the budget and the int32
// bound. All enum and boolean combinations are retained. An oversized proof
// fails closed instead of dropping middle pricing branches.
func proofVectors(expression string, schema map[string]jsplugin.UsageFieldSchema, budget int, hasBudget bool) ([]proofVector, error) {
	type dimension struct {
		name   string
		values []any
	}
	fields, err := seedancebilling.ReferencedUsageFields(expression, schema)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	dimensions := make([]dimension, 0, len(names))
	tokensValues := []int{0}
	if _, readsTokens := fields[seedancebilling.TokenUsageKey]; readsTokens {
		tokensValues = []int{0, 1, 120000, math.MaxInt32}
		if hasBudget && budget > 0 {
			tokensValues = append(tokensValues, budget)
			sort.Ints(tokensValues)
		}
	}
	for _, name := range names {
		field := fields[name]
		if name == seedancebilling.TokenUsageKey {
			continue
		}
		switch {
		case len(field.Enum) > 0:
			values := make([]any, 0, len(field.Enum))
			for _, value := range field.Enum {
				values = append(values, value)
			}
			dimensions = append(dimensions, dimension{name: name, values: values})
		case field.Type == "boolean":
			dimensions = append(dimensions, dimension{name: name, values: []any{false, true}})
		case field.Unit == "second":
			dimensions = append(dimensions, dimension{name: name, values: []any{float64(0), float64(1), float64(5), float64(3600)}})
		default:
			return nil, fmt.Errorf("field %q has no deterministic proof samples", name)
		}
	}

	combos := len(tokensValues)
	for _, dimension := range dimensions {
		if len(dimension.values) == 0 || combos > maxProofVectors/len(dimension.values) {
			return nil, fmt.Errorf("complete proof combinations exceed %d; narrow the supported contract for manual review", maxProofVectors)
		}
		combos *= len(dimension.values)
	}

	probeBodies := []map[string]any{{}}
	for _, dimension := range dimensions {
		next := make([]map[string]any, 0, len(probeBodies)*len(dimension.values))
		for _, probe := range probeBodies {
			for _, value := range dimension.values {
				extended := make(map[string]any, len(probe)+1)
				for key, element := range probe {
					extended[key] = element
				}
				extended[dimension.name] = value
				next = append(next, extended)
			}
		}
		probeBodies = next
	}

	vectors := make([]proofVector, 0, len(probeBodies)*len(tokensValues))
	for _, probe := range probeBodies {
		encoded, err := common.Marshal(map[string]any{"_task": probe})
		if err != nil {
			return nil, err
		}
		for _, tokens := range tokensValues {
			vectors = append(vectors, proofVector{ProbeBody: encoded, Tokens: tokens})
		}
	}
	return vectors, nil
}

// proveEquivalence re-evaluates both expressions over every vector and
// requires the final integer quota to match exactly under each group ratio.
// Floating-point tolerance is never accepted in place of funding equality.
func proveEquivalence(legacy, candidate string, vectors []proofVector, groupRatios []float64) string {
	if len(vectors) == 0 || len(groupRatios) == 0 {
		return "equivalence requires non-empty proof vectors and group ratios"
	}
	checkedCharges := map[float64]bool{}
	for _, vector := range vectors {
		legacyCost, legacyTrace, err := billingexpr.RunExprWithRequest(legacy, billingexpr.TokenParams{C: float64(vector.Tokens)}, billingexpr.RequestInput{Body: vector.ProbeBody})
		if err != nil {
			return fmt.Sprintf("legacy expression failed at tokens=%d: %v", vector.Tokens, err)
		}
		facts, err := seedancebilling.ControlledFacts(vector.ProbeBody, vector.Tokens)
		if err != nil {
			return fmt.Sprintf("candidate facts failed at tokens=%d: %v", vector.Tokens, err)
		}
		candidateCost, candidateTrace, err := billingexpr.RunExprWithRequest(candidate, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
		if err != nil {
			return fmt.Sprintf("candidate expression failed at tokens=%d: %v", vector.Tokens, err)
		}
		legacyUSD := legacyCost / 1_000_000
		if math.IsNaN(legacyUSD) || math.IsInf(legacyUSD, 0) || legacyUSD < 0 || legacyUSD != candidateCost {
			return fmt.Sprintf("model USD differs or is invalid at tokens=%d", vector.Tokens)
		}
		if legacyTrace.MatchedTier != candidateTrace.MatchedTier || !reflect.DeepEqual(legacyTrace.RequestRules, candidateTrace.RequestRules) {
			return "tier or request-rule trace differs; automatic migration cannot preserve this contract"
		}
		for _, groupRatio := range groupRatios {
			charge := legacyUSD * common.QuotaPerUnit * groupRatio
			// USD equality was checked for every vector. Repeated conditions with
			// the same charge need only one conversion check and saturation log.
			if checkedCharges[charge] {
				continue
			}
			checkedCharges[charge] = true
			legacyQuota, _ := common.QuotaRoundChecked(charge)
			candidateQuota, _ := common.QuotaRoundChecked(candidateCost * common.QuotaPerUnit * groupRatio)
			if legacyQuota != candidateQuota {
				return fmt.Sprintf("final quota differs at tokens=%d group_ratio=%g: legacy=%d candidate=%d", vector.Tokens, groupRatio, legacyQuota, candidateQuota)
			}
		}
	}
	return ""
}

func readOptions(db *gorm.DB) map[string]string {
	var rows []struct {
		Key   string
		Value string
	}
	if err := db.Table("options").Select("`key`", "value").Find(&rows).Error; err != nil {
		fail("read options", err)
	}
	options := map[string]string{}
	for _, row := range rows {
		options[row.Key] = row.Value
	}
	return options
}

func readSeedanceProtocols(db *gorm.DB) map[string]map[dto.VideoUpstreamProtocol]bool {
	var channels []model.Channel
	if err := db.Table("channels").Select("id", "type", "status", "models", "settings").
		Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		fail("read seedance channels", err)
	}
	protocols := map[string]map[dto.VideoUpstreamProtocol]bool{}
	for name, selected := range model.SeedancePricingChannels(channels) {
		protocols[name] = map[dto.VideoUpstreamProtocol]bool{}
		for _, channel := range selected {
			settings := dto.ChannelOtherSettings{}
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
				fail("read protocol", fmt.Errorf("invalid channel settings"))
			}
			if !settings.VideoUpstreamProtocol.IsValid() {
				fail("read protocol", fmt.Errorf("invalid Seedance protocol in snapshot"))
			}
			protocols[name][settings.VideoUpstreamProtocol] = true
		}
	}

	return protocols
}

// configuredValues mirrors the server's per-model configured pricing view so
// the emitted expected_version is computed from the same shape.
func configuredValues(options map[string]string, model string) map[string]any {
	values := map[string]any{}
	for _, key := range pricingOptionKeys {
		entries := decodeMap(options[key])
		if value, exists := entries[model]; exists {
			values[key] = value
		}
	}
	return values
}

func modelPricingVersion(values map[string]any) string {
	return model.ModelPricingVersion(model.PricingValues(values))
}

func decodeMap(raw string) map[string]any {
	entries := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return entries
	}
	if err := common.UnmarshalJsonStr(raw, &entries); err != nil {
		fail("decode pricing option", fmt.Errorf("invalid option JSON"))
	}
	return entries
}

func intValue(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 || number > float64(billing_setting.MaxTaskPreConsumeTokens) || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

func sortedKeys[T any](set map[string]T) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedProtocolNames(set map[dto.VideoUpstreamProtocol]bool) []string {
	out := make([]string, 0, len(set))
	for protocol := range set {
		out = append(out, string(protocol))
	}
	sort.Strings(out)
	return out
}

func restoreModels(entries []restoreEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.ModelName)
	}
	return out
}

func containsOutcome(lines []migrationLine, outcome string) bool {
	for _, line := range lines {
		if line.Outcome == outcome {
			return true
		}
	}
	return false
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", stage, err)
	os.Exit(1)
}
