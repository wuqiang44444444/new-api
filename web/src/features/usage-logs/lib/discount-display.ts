import {
  combinedDiscountFactor,
  formatDiscountFactor,
  formatDiscountTier,
} from '@/features/billing-reconciliation/lib'

import type { LogOtherData } from '../types'

type Translate = (key: string, opts?: Record<string, unknown>) => string

export type DiscountDisplay = {
  visible: boolean
  groupTier: string
  groupFactor: string
  contractState: 'applied' | 'not_applied' | 'unrecorded'
  contractLabel: string
  contractTier: string
  contractFactor: string
  finalTier: string
  finalFactor: string
}

export function getContractDiscountFact(other: LogOtherData | null): {
  state: DiscountDisplay['contractState']
  ratio: number | null
} {
  // 已由后端认定为未知的历史事实不按普通日志的无合同语义解释。
  if (other?.billing_facts?.contract_applicable === 'unknown') {
    return { state: 'unrecorded', ratio: null }
  }
  const ratio = Number(other?.contract_discount)
  const hasRatio = Number.isFinite(ratio) && ratio > 0
  if (other?.contract_applicable === false) {
    return { state: hasRatio ? 'unrecorded' : 'not_applied', ratio: null }
  }
  if (hasRatio) {
    return { state: 'applied', ratio }
  }
  const hasContractEvidence =
    other?.contract_applicable === true ||
    other?.contract_discount != null ||
    (other?.contract_id ?? 0) > 0 ||
    (other?.contract_version ?? 0) > 0 ||
    Boolean(other?.contract_name?.trim())
  return {
    state: hasContractEvidence ? 'unrecorded' : 'not_applied',
    ratio: null,
  }
}

// 使用记录的分组/合同/最终折扣展示（方案 §2）。倍率只显示当次实际生效的
// 一项（用户专属优先，不叠乘）；普通日志无合同字段时按无合同显示，合同因子为 1。
// 明确未知或存在不完整/冲突合同事实时继续保留未知。
export function buildDiscountDisplay(
  other: LogOtherData | null,
  translate: Translate
): DiscountDisplay {
  const userExclusive =
    other?.user_group_ratio != null && other.user_group_ratio !== -1
  const groupRatio = userExclusive
    ? other?.user_group_ratio
    : (other?.group_ratio ?? null)
  const { state: contractState, ratio: contractRatio } =
    getContractDiscountFact(other)
  const contractApplied = contractState === 'applied'
  const explicitNotApplied = other?.contract_applicable === false

  const groupTier =
    groupRatio != null && groupRatio > 0
      ? formatDiscountTier(groupRatio, translate)
      : ''
  const groupFactor =
    groupRatio != null && groupRatio > 0 ? formatDiscountFactor(groupRatio) : ''
  const contractTier =
    contractRatio != null ? formatDiscountTier(contractRatio, translate) : ''
  const contractFactor =
    contractRatio != null ? formatDiscountFactor(contractRatio) : ''

  let contractLabel: string
  if (contractState === 'not_applied') {
    contractLabel =
      other?.billing_facts?.contract_applicable === 'unrecorded'
        ? translate('Not recorded')
        : translate('No contract')
  } else if (contractState === 'unrecorded') {
    contractLabel = translate('Contract discount: not recorded')
  } else if (other?.contract_name) {
    contractLabel = other.contract_name
  } else if (other?.contract_version != null && other.contract_version > 0) {
    contractLabel = translate('Historical identity not recorded')
  } else {
    contractLabel = translate('Contract discount: not recorded')
  }

  // 最终折扣只在组倍率已知、且合同状态明确（生效或明确未适用）时给出。
  let contractFactorForFinal: number | null = null
  if (contractState === 'applied') {
    contractFactorForFinal = contractRatio
  } else if (contractState === 'not_applied') {
    contractFactorForFinal = 1
  }
  const finalRatio = combinedDiscountFactor(groupRatio, contractFactorForFinal)
  const finalTier =
    finalRatio != null ? formatDiscountTier(finalRatio, translate) : ''
  const finalFactor = finalRatio != null ? formatDiscountFactor(finalRatio) : ''

  const visible =
    groupRatio != null ||
    contractApplied ||
    explicitNotApplied ||
    (other?.contract_name != null && other.contract_name !== '')

  return {
    visible,
    groupTier,
    groupFactor,
    contractState,
    contractLabel,
    contractTier,
    contractFactor,
    finalTier,
    finalFactor,
  }
}
