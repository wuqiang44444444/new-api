package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PricingValues is one model's configuration, keyed by the existing option
// names. A missing key inherits the engine's default; an explicit zero is free.
type PricingValues map[string]any

type ModelPricingChange struct {
	ModelName       string        `json:"model_name"`
	ExpectedVersion string        `json:"expected_version"`
	Pricing         PricingValues `json:"pricing"`
	Reset           bool          `json:"reset,omitempty"`
}

type ModelPricingEntry struct {
	BillingDisplay *billingexpr.DisplayProjection       `json:"billing_display,omitempty"`
	ModelName      string                               `json:"model_name"`
	Version        string                               `json:"version"`
	Configured     PricingValues                        `json:"configured"`
	Effective      PricingValues                        `json:"effective"`
	UsageSchema    map[string]jsplugin.UsageFieldSchema `json:"usage_schema,omitempty"`
	// Seedance Link attribution uses the same typed facts as the public
	// pricing surface: a cross-channel conflict blocks editing and saving on
	// both; a Seedance usage schema marks the declared u() field contract the
	// task usage editor consumes together with the separate token-budget
	// field.
	BillingContractConflict bool `json:"billing_contract_conflict,omitempty"`
	PreconsumeTokenBudget   bool `json:"preconsume_token_budget,omitempty"`
}

type ModelPricingSnapshot struct {
	Entries      []ModelPricingEntry `json:"entries"`
	Options      map[string]string   `json:"options"`
	EmptyVersion string              `json:"empty_version"`
}

var ErrModelPricingConflict = errors.New("model pricing changed; reload before saving")

// Lock order is stable across instances. Creating missing option rows inside
// the transaction also serializes the first write to an unconfigured database.
var modelPricingOptionKeys = []string{
	"AudioCompletionRatio", "AudioRatio", "CacheRatio", "CompletionRatio",
	"CreateCacheRatio", "ImageRatio", "ModelPrice", "ModelRatio",
	"billing_setting.billing_expr", "billing_setting.billing_mode",
	billing_setting.TaskPreConsumeTokensOption,
}

var modelPricingMutationMu sync.Mutex

func IsModelPricingOption(key string) bool {
	return slices.Contains(modelPricingOptionKeys, key)
}

func ModelPricingVersion(values PricingValues) string {
	encoded, _ := common.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func defaultPricingMaps() map[string]map[string]any {
	result := make(map[string]map[string]any, len(modelPricingOptionKeys))
	for _, key := range modelPricingOptionKeys {
		result[key] = make(map[string]any)
	}
	for key, values := range ratio_setting.GetDefaultPricingMaps() {
		for name, value := range values {
			result[key][name] = value
		}
	}
	return result
}

func readModelPricingMaps(db *gorm.DB) (map[string]map[string]any, error) {
	var rows []Option
	if err := db.Where(commonKeyCol+" IN ?", modelPricingOptionKeys).Find(&rows).Error; err != nil {
		return nil, err
	}
	values := defaultPricingMaps()
	for _, row := range rows {
		var entries map[string]any
		if err := common.UnmarshalJsonStr(row.Value, &entries); err != nil {
			return nil, fmt.Errorf("%s: %w", row.Key, err)
		}
		if entries == nil {
			return nil, fmt.Errorf("%s must be a JSON object", row.Key)
		}
		values[row.Key] = entries
	}
	return values, nil
}

func modelPricingValues(values map[string]map[string]any, name string) PricingValues {
	result := make(PricingValues)
	for _, key := range modelPricingOptionKeys {
		if value, exists := values[key][name]; exists {
			result[key] = value
		}
	}
	return result
}

func effectiveModelPricing(values map[string]map[string]any, name string) PricingValues {
	result := modelPricingValues(values, name)
	// Legacy wildcard aliases are resolved by the same normalization as relay.
	alias := ratio_setting.FormatMatchingModelName(name)
	for _, key := range modelPricingOptionKeys[:8] {
		if value, exists := values[key][alias]; exists {
			result[key] = value
		}
	}
	mode, _ := result["billing_setting.billing_mode"].(string)
	if mode == "" {
		_, hasPrice := result["ModelPrice"]
		_, hasRatio := result["ModelRatio"]
		if _, builtin := billing_setting.GetBuiltinBillingExpr(name); builtin && !hasPrice && !hasRatio {
			mode = "tiered_expr"
		}
	}
	if mode == "tiered_expr" {
		result["billing_setting.billing_mode"] = mode
		if _, exists := result["billing_setting.billing_expr"]; !exists {
			if expression, ok := billing_setting.GetBuiltinBillingExpr(name); ok {
				result["billing_setting.billing_expr"] = expression
			}
		}
		return result
	}
	if _, exists := result["ModelPrice"]; exists {
		return result
	}
	if _, exists := result["ModelRatio"]; !exists && operation_setting.SelfUseModeEnabled {
		result["ModelRatio"] = float64(37.5)
	}
	// Completion ratios include engine-enforced model defaults. Expose their
	// effective value without persisting them into the editable configuration.
	completion := ratio_setting.GetCompletionRatioInfo(name)
	if _, exists := result["CompletionRatio"]; !exists || completion.Locked {
		result["CompletionRatio"] = completion.Ratio
	}
	return result
}

func GetModelPricingSnapshot(names []string) (*ModelPricingSnapshot, error) {
	values, err := readModelPricingMaps(DB)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		nameSet := make(map[string]bool)
		for _, entries := range values {
			for name := range entries {
				nameSet[name] = true
			}
		}
		for name := range billing_setting.GetBuiltinBillingExprCopy() {
			nameSet[name] = true
		}
		for name := range nameSet {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	attribution, err := loadSeedancePricingAttribution(DB)
	if err != nil {
		return nil, err
	}
	result := &ModelPricingSnapshot{Entries: make([]ModelPricingEntry, 0, len(names)), Options: make(map[string]string), EmptyVersion: ModelPricingVersion(PricingValues{})}
	generation := jsplugin.DefaultRegistry.Generation()
	for _, name := range names {
		configured := modelPricingValues(values, name)
		entry := ModelPricingEntry{ModelName: name, Version: ModelPricingVersion(configured), Configured: configured, Effective: effectiveModelPricing(values, name)}
		// Seedance 归属优先于通用插件模型索引或 alias：客户模型已由类型化渠道归
		// Seedance 时，管理端与公开端一致使用 Seedance 字段合同，原生 usage_schema
		// 不得使 UI 进入原生任务编辑器（本次故障根因）。
		if schema, ok := attribution.schemas[name]; ok {
			entry.UsageSchema = schema
			_, entry.PreconsumeTokenBudget = schema["tokens"]
			entry.BillingContractConflict = attribution.pricingContractConflict(name)
		} else if plugin, ok := generation.GetByModel(name); ok {
			entry.UsageSchema = plugin.Meta.UsageSchema
		} else if target, ok := ResolveTaskModelAlias(generation, name); ok {
			if plugin, ok := generation.Get(target.PluginKey); ok {
				entry.UsageSchema = plugin.Meta.UsageSchema
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	// Preserve the existing settings editor's full-map interface. Built-in
	// expressions are display defaults only; per-model writes do not persist them.
	for name, expression := range billing_setting.GetBuiltinBillingExprCopy() {
		effective := effectiveModelPricing(values, name)
		if effective["billing_setting.billing_mode"] != "tiered_expr" {
			continue
		}
		if _, ok := values["billing_setting.billing_mode"][name]; !ok {
			values["billing_setting.billing_mode"][name] = "tiered_expr"
		}
		if _, ok := values["billing_setting.billing_expr"][name]; !ok {
			values["billing_setting.billing_expr"][name] = expression
		}
	}
	for key, entries := range values {
		encoded, err := common.Marshal(entries)
		if err != nil {
			return nil, err
		}
		result.Options[key] = string(encoded)
	}
	return result, nil
}

func ValidateModelPricing(name string, values PricingValues) error {
	return validateModelPricing(DB, name, values)
}

func validateModelPricing(db *gorm.DB, name string, values PricingValues) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("model name is required")
	}
	for key, value := range values {
		if !IsModelPricingOption(key) {
			return fmt.Errorf("unsupported pricing field: %s", key)
		}
		if key == "billing_setting.billing_mode" {
			if value != "ratio" && value != "tiered_expr" {
				return errors.New("invalid billing mode")
			}
			continue
		}
		if key == "billing_setting.billing_expr" {
			expression, ok := value.(string)
			if !ok || strings.TrimSpace(expression) == "" {
				return errors.New("billing expression is required")
			}
			if handled, err := validateSeedanceBillingExpression(db, name, expression); handled {
				if err != nil {
					return err
				}
				continue
			}
			generation := jsplugin.DefaultRegistry.Generation()
			var err error
			if plugin, ok := generation.GetByModel(name); ok {
				err = billing_setting.SmokeTestTaskExpr(expression, plugin.Meta.UsageSchema)
			} else if target, resolved := ResolveTaskModelAlias(generation, name); resolved {
				if plugin, ok := generation.Get(target.PluginKey); ok {
					err = billing_setting.SmokeTestTaskExpr(expression, plugin.Meta.UsageSchema)
				} else {
					err = billing_setting.SmokeTestExpr(expression)
				}
			} else {
				err = billing_setting.SmokeTestExpr(expression)
			}
			if err != nil {
				return fmt.Errorf("model %s: %w", name, err)
			}
			continue
		}
		if key == billing_setting.TaskPreConsumeTokensOption {
			encoded, err := common.Marshal(map[string]any{name: value})
			if err != nil {
				return err
			}
			if err := billing_setting.ValidateTaskPreConsumeTokensJSON(string(encoded)); err != nil {
				return err
			}
			continue
		}
		number, ok := value.(float64)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return fmt.Errorf("%s must be a finite, non-negative number", key)
		}
	}
	if values["billing_setting.billing_mode"] == "tiered_expr" {
		if _, exists := values["billing_setting.billing_expr"]; !exists {
			if _, builtin := billing_setting.GetBuiltinBillingExpr(name); !builtin {
				return errors.New("billing expression is required")
			}
		}
	}
	return validateSeedancePricingConfiguration(db, name, values)
}

func UpdateModelPricing(changes []ModelPricingChange) error {
	if len(changes) == 0 {
		return errors.New("select model pricing changes before saving")
	}
	seen := make(map[string]bool)
	for _, change := range changes {
		if seen[change.ModelName] {
			return errors.New("duplicate model pricing change")
		}
		seen[change.ModelName] = true
		if change.ExpectedVersion == "" {
			return ErrModelPricingConflict
		}
	}
	return mutateModelPricingOptions(func(tx *gorm.DB, values map[string]map[string]any) error {
		defaults := defaultPricingMaps()
		for _, change := range changes {
			if ModelPricingVersion(modelPricingValues(values, change.ModelName)) != change.ExpectedVersion {
				return fmt.Errorf("%w: %s", ErrModelPricingConflict, change.ModelName)
			}
			pricing := change.Pricing
			if change.Reset {
				pricing = modelPricingValues(defaults, change.ModelName)
			}
			if err := validateModelPricing(tx, change.ModelName, pricing); err != nil {
				return err
			}
			for _, key := range modelPricingOptionKeys {
				delete(values[key], change.ModelName)
				if value, exists := pricing[key]; exists {
					values[key][change.ModelName] = value
				}
			}
		}
		return nil
	})
}

// UpdateModelPricingOptions keeps legacy single-option callers on the same
// locking, validation and transaction path as the model-level API.
func UpdateModelPricingOptions(updates map[string]string) error {
	return mutateModelPricingOptions(func(tx *gorm.DB, values map[string]map[string]any) error {
		names := make(map[string]bool)
		for key, raw := range updates {
			if !IsModelPricingOption(key) {
				return fmt.Errorf("unsupported pricing field: %s", key)
			}
			var entries map[string]any
			if err := common.UnmarshalJsonStr(raw, &entries); err != nil {
				return err
			}
			if entries == nil {
				return fmt.Errorf("%s must be a JSON object", key)
			}
			for name := range values[key] {
				names[name] = true
			}
			values[key] = entries
			for name := range entries {
				names[name] = true
			}
		}
		for name := range names {
			if err := validateModelPricing(tx, name, modelPricingValues(values, name)); err != nil {
				return err
			}
		}
		return nil
	})
}

func mutateModelPricingOptions(mutate func(*gorm.DB, map[string]map[string]any) error) error {
	modelPricingMutationMu.Lock()
	defer modelPricingMutationMu.Unlock()
	var committed map[string]map[string]any
	err := DB.Transaction(func(tx *gorm.DB) error {
		defaults := defaultPricingMaps()
		for _, key := range modelPricingOptionKeys {
			encoded, err := common.Marshal(defaults[key])
			if err != nil {
				return err
			}
			row := Option{Key: key, Value: string(encoded)}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		values, err := readModelPricingMaps(lockForUpdate(tx))
		if err != nil {
			return err
		}
		if err := mutate(tx, values); err != nil {
			return err
		}
		for _, key := range modelPricingOptionKeys {
			encoded, err := common.Marshal(values[key])
			if err != nil {
				return err
			}
			if err := tx.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", string(encoded)).Error; err != nil {
				return err
			}
		}
		committed = values
		return nil
	})
	if err != nil {
		return err
	}
	for _, key := range modelPricingOptionKeys {
		encoded, _ := common.Marshal(committed[key])
		if err := updateOptionMap(key, string(encoded)); err != nil {
			return err
		}
	}
	RefreshPricing()
	ratio_setting.InvalidateExposedDataCache()
	return nil
}
