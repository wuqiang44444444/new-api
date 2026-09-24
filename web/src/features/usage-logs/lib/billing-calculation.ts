import type { TFunction } from 'i18next'

export interface BillingCalculation {
  version: number
  quota: number
  matched_tier?: string
  nodes?: { id: number; op: string; args?: number[]; literal?: string }[]
  values?: { node: number; value: string }[]
  steps?: {
    op: string
    formula: string
    inputs?: string[]
    result: string
    unit?: string
  }[]
}

export interface TaskCalculation {
  quota: number
  charge_unknown?: boolean
  refunded_quota?: number
  waived_quota?: number
  state: string
  source: string
  evidence: string
  initial_evidence: string
  initial?: BillingCalculation
  settlement?: BillingCalculation
  target_quota?: number
}

// Formatting only: all values and results come from the saved evaluation.
export function calculationFormula(
  op: string,
  args: string[],
  t: TFunction
): string {
  const a = args
  if (op.startsWith('usage:')) {
    return `${t('Billing quantity')} (${op.slice(6)})`
  }

  switch (op) {
    case 'unary:-':
      return `−${a[0]}`
    case 'unary:+':
      return `+${a[0]}`
    case 'unary:!':
    case 'unary:not':
      return `not(${a[0]})`
    case 'integer_division':
      return `${t('Integer division')}(${a[0]} ÷ ${a[1]})`
    case '+':
      return a.join(' + ')
    case '-':
      return a.join(' − ')
    case '*':
      return a.join(' × ')
    case '/':
      return a.join(' ÷ ')
    case '>':
    case '>=':
    case '<':
    case '<=':
    case '==':
    case '!=':
    case '&&':
    case '||':
    case 'and':
    case 'or':
    case '**':
      return a.join(` ${op} `)
    case '%':
      return `${a[0]} mod ${a[1]}`
    case 'if':
      return `${t('Condition')} ${a[0]} → ${a[0] === 'true' ? a[1] : a[2]} (${t('Other branch not executed')})`
    case 'p':
      return t('Normalized input tokens')
    case 'c':
      return t('Normalized output tokens')
    case 'len':
      return t('Input context length')
    case 'cr':
      return t('Cached Tokens')
    case 'cc':
    case 'cc1h':
      return `${t('Cache creation tokens')} (${op})`
    case 'img':
    case 'img_o':
      return `${t('Image tokens')} (${op})`
    case 'ai':
    case 'ao':
      return `${t('Audio tokens')} (${op})`
    case 'tier':
      return `${t('Tier price')}: ${a[1]}`
    case 'param':
    case 'header':
    case 'protected_request':
      return t('Request billing value')
    case 'usd_exchange_rate':
      return t('Frozen CNY per USD exchange rate')
    case '_trace':
    case '_trace_int':
      return `${t('Condition')} ${a[1]} → ${a[1] === 'true' ? a[2] : '1'}`
    default:
      return `${op}(${a.join(', ')})`
  }
}

export function recordedExpressionRows(
  calculation: BillingCalculation,
  t: TFunction
) {
  const nodes = new Map(calculation.nodes?.map((node) => [node.id, node]))
  const values = new Map<number, string>()
  for (const node of nodes.values()) {
    if (node.literal !== undefined) values.set(node.id, node.literal)
  }
  const display = (value: string) =>
    value === 'protected' ? t('Protected condition value') : value
  return (calculation.values ?? []).map((entry) => {
    const node = nodes.get(entry.node)
    const args =
      node?.args?.map((id, index) => {
        const left = values.get(node.args?.[0] ?? -1)
        const skipped =
          index === 1 &&
          ((['&&', 'and'].includes(node.op) && left === 'false') ||
            (['||', 'or'].includes(node.op) && left === 'true') ||
            (node.op === '??' && left !== undefined && left !== 'null'))
        if (skipped) return `#${id} (${t('Not executed')})`
        return display(
          values.get(id) ??
            (['predicate', 'pair', 'let', 'member'].includes(
              nodes.get(id)?.op ?? ''
            )
              ? `#${id}`
              : `#${id} (${t('Not executed')})`)
        )
      }) ?? []
    values.set(entry.node, entry.value)
    return {
      id: entry.node,
      op: node?.op ?? '',
      rawResult: entry.value,
      formula: calculationFormula(node?.op ?? '', args, t),
      result: display(entry.value),
    }
  })
}
