package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 受控合同拒绝诊断（docs/80-dev/2026-09-22-近三天数据库错误分析与代码改进建议.md P1-1）：
// 阶段与原因码全部来自既有判断分支，仅合并进统一错误事件，不参与权限判定，
// 不改变公开响应。Attach 合并语义保证同一次请求仍只产生一条最终事件。
const (
	contractStageScope     = "contract_scope"
	contractStageSelection = "channel_selection"

	contractReasonUnavailable           = "contract_unavailable"
	contractReasonModelNotListed        = "contract_model_not_listed"
	contractReasonRouteLookupFailed     = "contract_route_lookup_failed"
	contractReasonCandidatesUnavailable = "contract_candidates_unavailable"
	contractReasonNoSelectableChannel   = "contract_no_selectable_channel"
	contractReasonChannelNotInScope     = "contract_channel_not_in_scope"
	contractReasonChannelMismatch       = "contract_channel_mismatch"
)

// attachContractRejection records one contract rejection observation into the
// request-scoped event carrier; missing fields are filled by later merge
// points. The carrier lives on the request context (gin context values do not
// reach it without ContextWithFallback), so Attach receives the request context.
func attachContractRejection(c *gin.Context, stage, reason, publicModel string, detail map[string]string) {
	if c == nil || c.Request == nil {
		return
	}
	if detail == nil {
		detail = map[string]string{}
	}
	if snapshot := ActiveCustomerContract(c); snapshot != nil {
		detail["contract_id"] = strconv.Itoa(snapshot.Id)
		detail["contract_version"] = strconv.FormatInt(snapshot.Version, 10)
		if _, present := detail["candidates_total"]; !present {
			count := 0
			for _, rule := range snapshot.Rules {
				if rule.PublicModel == publicModel {
					count++
				}
			}
			detail["candidates_total"] = strconv.Itoa(count)
		}
	}
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Stage:  stage,
		Reason: reason,
		Model:  strings.TrimSpace(publicModel),
		Detail: detail,
	})
}

// contractCandidatesDetail 汇总候选规则的受控不可用类别，产出有界诊断明细：
// 候选总数与按类别计数（如 channel_disabled=2|capability_missing=1）。
func contractCandidatesDetail(contract *model.ContractEntitySnapshot, rules []model.ContractEntityRule) map[string]string {
	detail := map[string]string{}
	if contract != nil {
		detail["contract_id"] = strconv.Itoa(contract.Id)
		detail["contract_version"] = strconv.FormatInt(contract.Version, 10)
	}
	blocked := make(map[string]int)
	for _, rule := range rules {
		if rule.Available {
			continue
		}
		category := rule.UnavailableCategory
		if category == "" {
			category = model.ContractRouteUnavailableOther
		}
		blocked[category]++
	}
	detail["candidates_total"] = strconv.Itoa(len(rules))
	if len(blocked) == 0 {
		return detail
	}
	categories := make([]string, 0, len(blocked))
	for category, count := range blocked {
		categories = append(categories, fmt.Sprintf("%s=%d", category, count))
	}
	sort.Strings(categories)
	detail["candidates_blocked"] = strings.Join(categories, "|")
	return detail
}
