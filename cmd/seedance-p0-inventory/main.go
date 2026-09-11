// seedance-p0-inventory 生成 Seedance 专用渠道的脱敏 P0 盘点矩阵
// （方案 §8.1）：全部启用与停用 62 渠道的客户模型、映射结果、视频/素材协议、
// 普通素材组策略、定价模式、用量依赖、预扣状态与表达式可编译性，并单独列出
// 跨原生/Link 价格键冲突、mode/expr 不一致、预扣缺失与无渠道引用的价格条目。
//
// 工具只读取本地数据库快照，按构造脱敏：不查询密钥列，不输出 Base URL、
// 代理、Provider Project/Region、素材 ID 或 Task 私有数据。
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type channelRow struct {
	Id           int    `gorm:"column:id"`
	Type         int    `gorm:"column:type"`
	Status       int    `gorm:"column:status"`
	Group        string `gorm:"column:group"`
	Models       string `gorm:"column:models"`
	ModelMapping string `gorm:"column:model_mapping"`
	Settings     string `gorm:"column:settings"`
}

type taskStatusCount struct {
	Status       string
	BillingState string
	Count        int
}

type pluginRow struct {
	Key     string
	Version string
	Active  bool
	Enabled bool
}

type modelLine struct {
	Channel      int
	Status       int
	Model        string
	Mapped       string
	Mode         string
	ExprOK       bool
	UsageDep     bool
	Preconsume   bool
	ChannelRefer bool
}

func main() {
	dsn := flag.String("dsn", "", "SQLite snapshot path (read-only)")
	checkArtifacts := flag.Bool("check-plugin-artifacts", false, "check every stored Seedance version against the current compiler only")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: seedance-p0-inventory -dsn <sqlite file>")
		os.Exit(2)
	}
	db, err := gorm.Open(sqlite.Open(*dsn+"?mode=ro"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		fail("open snapshot", err)
	}
	if *checkArtifacts {
		if err := checkSeedancePluginArtifacts(db, os.Stdout); err != nil {
			fail("check plugin artifacts", err)
		}
		return
	}

	var channels []channelRow
	if err = db.Table("channels").
		Select("id", "type", "status", "group", "models", "model_mapping", "settings").
		Order("id").Find(&channels).Error; err != nil {
		fail("read channels", err)
	}

	options := map[string]string{}
	var optionRows []struct {
		Key   string
		Value string
	}
	if err = db.Table("options").Select("`key`", "value").Find(&optionRows).Error; err != nil {
		fail("read options", err)
	}
	for _, row := range optionRows {
		options[row.Key] = row.Value
	}

	mode := map[string]string{}
	_ = common.UnmarshalJsonStr(options["billing_setting.billing_mode"], &mode)
	expr := map[string]string{}
	_ = common.UnmarshalJsonStr(options["billing_setting.billing_expr"], &expr)
	preconsume := map[string]int{}
	_ = common.UnmarshalJsonStr(options["task_billing_setting.preconsume_tokens"], &preconsume)
	modelPrice := map[string]float64{}
	_ = common.UnmarshalJsonStr(options["ModelPrice"], &modelPrice)
	modelRatio := map[string]float64{}
	_ = common.UnmarshalJsonStr(options["ModelRatio"], &modelRatio)

	var statusCounts []taskStatusCount
	if err = db.Table("tasks").
		Select("status", "COALESCE(billing_state,'') AS billing_state", "COUNT(*) AS count").
		Where("platform = ?", "62").
		Group("status").Group("billing_state").
		Find(&statusCounts).Error; err != nil {
		fail("read tasks", err)
	}

	var plugins []pluginRow
	if err = db.Table("task_plugins").
		Select("key", "version", "active", "enabled").Find(&plugins).Error; err != nil {
		fail("read task_plugins", err)
	}

	contractModels := map[string]bool{}
	var contractRows []struct {
		PublicModel string `gorm:"column:public_model"`
	}
	if err = db.Table("customer_model_contracts").
		Select("DISTINCT public_model").Find(&contractRows).Error; err != nil {
		fail("read contracts", err)
	}
	for _, row := range contractRows {
		contractModels[row.PublicModel] = true
	}

	defaultGroups := int64(0)
	if err = db.Table("channel_default_asset_groups").Count(&defaultGroups).Error; err != nil {
		fail("read default asset groups", err)
	}

	linkModels := map[string]bool{}
	nativeModels := map[string]bool{}
	onChannel := map[string]bool{}
	mappingTarget := map[string]bool{}
	for _, ch := range channels {
		for _, model := range splitModels(ch.Models) {
			onChannel[model] = true
			if ch.Type == 62 {
				linkModels[model] = true
			} else {
				nativeModels[model] = true
			}
		}
		mapping := map[string]string{}
		if ch.ModelMapping != "" {
			_ = common.UnmarshalJsonStr(ch.ModelMapping, &mapping)
		}
		for target := range mapping {
			mappingTarget[target] = true
		}
	}

	var lines []modelLine
	exprFailures := []string{}
	modeWithoutExpr := []string{}
	unpriced := []string{}
	preconsumeMissing := map[string]bool{}
	for _, ch := range channels {
		if ch.Type != 62 {
			continue
		}
		mapping := map[string]string{}
		if ch.ModelMapping != "" {
			_ = common.UnmarshalJsonStr(ch.ModelMapping, &mapping)
		}
		for _, model := range splitModels(ch.Models) {
			line := modelLine{
				Channel: ch.Id, Status: ch.Status, Model: model, Mapped: mapping[model],
			}
			exprKey := model
			if line.Mode = mode[model]; line.Mode == "" && line.Mapped != "" {
				// 与请求路径一致：客户模型未配置时按映射尾名回退。
				if line.Mode = mode[line.Mapped]; line.Mode != "" {
					exprKey = line.Mapped
				}
			}
			switch line.Mode {
			case "tiered_expr":
				expression := expr[exprKey]
				line.ExprOK = expression != ""
				if line.ExprOK {
					if _, err = billingexpr.CompileFromCache(expression); err != nil {
						line.ExprOK = false
						exprFailures = append(exprFailures, fmt.Sprintf("%d/%s: %v", ch.Id, model, err))
					}
					line.UsageDep = billingexpr.RequiresUsage(expression)
				}
			case "":
				if line.Mapped != "" && mode[line.Mapped] == "tiered_expr" {
					line.Mode = "tiered_expr(tail)"
					expression := expr[line.Mapped]
					line.ExprOK = expression != ""
					line.UsageDep = line.ExprOK && billingexpr.RequiresUsage(expression)
				}
			default:
				line.Mode = "ratio"
			}
			if line.Mode == "" || line.Mode == "ratio" {
				_, hasPrice := modelPrice[model]
				_, hasRatio := modelRatio[model]
				line.ExprOK = hasPrice || hasRatio
				if !line.ExprOK {
					unpriced = append(unpriced, fmt.Sprintf("%d/%s", ch.Id, model))
				}
			}
			if strings.HasPrefix(line.Mode, "tiered_expr") {
				_, ok := preconsume[model]
				if !ok && line.Mapped != "" {
					_, ok = preconsume[line.Mapped]
				}
				line.Preconsume = ok
				if !ok {
					preconsumeMissing[model] = true
				}
			}
			lines = append(lines, line)
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Channel != lines[j].Channel {
			return lines[i].Channel < lines[j].Channel
		}
		return lines[i].Model < lines[j].Model
	})

	conflicts := []string{}
	for model := range linkModels {
		if nativeModels[model] {
			conflicts = append(conflicts, model)
		}
	}
	sort.Strings(conflicts)
	for model := range mode {
		if expr[model] == "" {
			modeWithoutExpr = append(modeWithoutExpr, model)
		}
	}
	sort.Strings(modeWithoutExpr)

	unreferenced := []string{}
	appendUnreferenced := func(table string, keys []string) {
		for _, key := range keys {
			if !onChannel[key] && !mappingTarget[key] {
				unreferenced = append(unreferenced, table+":"+key)
			}
		}
	}
	appendUnreferenced("ModelPrice", sortedKeyList(modelPrice))
	appendUnreferenced("ModelRatio", sortedKeyList(modelRatio))
	appendUnreferenced("preconsume", sortedKeyList(preconsume))
	sort.Strings(unreferenced)

	fmt.Println("== 渠道矩阵（62 专用渠道，启用+停用）==")
	fmt.Println("channel | status | group | protocol | policy | default_group_present")
	for _, ch := range channels {
		if ch.Type != 62 {
			continue
		}
		settings := dto.ChannelOtherSettings{}
		_ = common.UnmarshalJsonStr(ch.Settings, &settings)
		fmt.Printf("%d | %s | %s | %s / %s | %s | %s\n",
			ch.Id, channelStatus(ch.Status), ch.Group,
			protocolValue(string(settings.VideoUpstreamProtocol)), protocolValue(string(settings.AssetUpstreamProtocol)),
			settings.AssetUpstreamProtocol.GeneralAssetGroupPolicy(),
			yesno(defaultGroups > 0 && hasDefaultGroup(db, ch.Id)))
	}
	fmt.Println()
	fmt.Println("== 模型定价矩阵 ==")
	fmt.Println("channel | status | model | mapped | mode | expr_ok | usage_dependent | preconsume")
	for _, line := range lines {
		fmt.Printf("%d | %s | %s | %s | %s | %s | %s | %s\n",
			line.Channel, channelStatus(line.Status), line.Model,
			valueOr(line.Mapped, "-"), valueOr(line.Mode, "NONE"),
			yesno(line.ExprOK), yesno(line.UsageDep), triPreconsume(line))
	}
	fmt.Println()
	fmt.Println("== 特殊记录（有待处理项的线路不进入切换批次）==")
	fmt.Printf("跨原生/Link 价格键冲突: %s\n", listOrNone(conflicts))
	fmt.Printf("tiered 模式缺表达式: %s\n", listOrNone(modeWithoutExpr))
	fmt.Printf("表达式编译失败: %s\n", listOrNone(exprFailures))
	fmt.Printf("无价格条目模型: %s\n", listOrNone(dedupe(unpriced)))
	fmt.Printf("tiered 模型缺预扣: %s\n", listOrNone(sortedKeys(preconsumeMissing)))
	fmt.Printf("无渠道引用的价格条目: %s\n", listOrNone(unreferenced))
	fmt.Printf("客户合同覆盖的 Link 模型: %s\n", listOrNone(intersect(contractModels, linkModels)))
	fmt.Println()
	fmt.Println("== 存量事实 ==")
	fmt.Println("tasks(62) status × billing_state:")
	for _, row := range statusCounts {
		fmt.Printf("  %s | %s | %d\n", row.Status, stateOrEmpty(row.BillingState), row.Count)
	}
	fmt.Println("task_plugins:")
	for _, row := range plugins {
		fmt.Printf("  %s | %s | active=%s | enabled=%s\n", row.Key, row.Version, yesno(row.Active), yesno(row.Enabled))
	}
	if len(plugins) == 0 {
		fmt.Println("  (空：运行时 sync 首次启动自动 seed，无需手工迁移)")
	}
	fmt.Printf("channel_default_asset_groups 行数: %d\n", defaultGroups)
}

func splitModels(raw string) []string {
	models := strings.Split(raw, ",")
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}

func channelStatus(status int) string {
	switch status {
	case 1:
		return "enabled"
	case 2:
		return "disabled_manual"
	case 3:
		return "disabled_auto"
	default:
		return fmt.Sprintf("status_%d", status)
	}
}

func protocolValue(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "unset"
}

func hasDefaultGroup(db *gorm.DB, channelID int) bool {
	var count int64
	if err := db.Table("channel_default_asset_groups").Where("channel_id = ?", channelID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func intersect(left, right map[string]bool) []string {
	out := []string{}
	for key := range left {
		if right[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeyList[T any](set map[string]T) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(set map[string]bool) []string {
	return sortedKeyList(set)
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func listOrNone(values []string) string {
	if len(values) == 0 {
		return "无"
	}
	return strings.Join(values, ", ")
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func stateOrEmpty(state string) string {
	if state == "" {
		return "(empty)"
	}
	return state
}

func yesno(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func triPreconsume(line modelLine) string {
	if !strings.HasPrefix(line.Mode, "tiered_expr") {
		return "n/a"
	}
	return yesno(line.Preconsume)
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", stage, err)
	os.Exit(1)
}
