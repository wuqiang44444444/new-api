/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// ----------------------------------------------------------------------------
// Pricing Types
// ----------------------------------------------------------------------------

export type PricingVendor = {
  id: number
  name: string
  icon?: string
  description?: string
}

export type BillingUsageUnit = 'second' | 'count' | 'token' | 'credit'

export type BillingUsageFieldSchema = {
  type?: 'number' | 'boolean'
  unit?: BillingUsageUnit
  enum?: string[]
  enumLabels?: Record<string, string | Record<string, string>>
  description?: string | Record<string, string>
}

export type BillingUsageSchema = Record<string, BillingUsageFieldSchema>

export type BillingUsageExample = {
  label: string
  facts: Record<string, string | number>
  /** 后端按冻结表达式对 facts 求值得到的 USD 金额；缺失表示不可复算。 */
  total?: number
}

export type PricingModel = {
  id: number
  model_name: string
  description?: string
  icon?: string
  vendor_id?: number
  vendor_name?: string
  vendor_icon?: string
  vendor_description?: string
  quota_type: number
  model_ratio: number
  /** false 表示倍率基础价来自缺价默认值而非显式配置。 */
  basis_price_configured?: boolean
  completion_ratio: number
  model_price?: number
  cache_ratio?: number | null
  create_cache_ratio?: number | null
  image_ratio?: number | null
  audio_ratio?: number | null
  audio_completion_ratio?: number | null
  enable_groups: string[]
  tags?: string
  supported_endpoint_types?: string[]
  key?: string
  group_ratio?: Record<string, number>
  /** Billing mode (e.g. "tiered_expr") used to flag dynamic pricing */
  billing_mode?: string
  /** Raw expression describing dynamic / tiered billing */
  billing_expr?: string
  /** Backend-generated read-only display projection of `billing_expr`. */
  billing_display?: BillingDisplayProjection
  /** Different channel contracts have claimed the same customer price key. */
  billing_contract_conflict?: boolean
  /** Task-plugin usage facts and their billing units. */
  billing_usage_schema?: BillingUsageSchema
  /** Display-only labeled usage vectors for pricing examples. */
  billing_usage_examples?: BillingUsageExample[]
  /** Pricing version returned by backend, useful for cache busting */
  pricing_version?: string
  /**
   * Optional model metadata fields reserved for backend-provided catalog data.
   * Keep them data-driven; do not synthesize display values on the client.
   */
  context_length?: number
  max_output_tokens?: number
  knowledge_cutoff?: string
  release_date?: string
  parameter_count?: string
  input_modalities?: Modality[]
  output_modalities?: Modality[]
  capabilities?: ModelCapability[]
  available?: boolean
  availability?: string
  api?: {
    assets?: {
      supported?: boolean
      reuse_scope?: string
    }
  }
  asset_share_group?: {
    label: string
    models: string[]
  }
}

/** Input/output modalities supported by a model. */
export type Modality = 'text' | 'image' | 'audio' | 'video' | 'file'

/** Functional capabilities a model exposes. */
export type ModelCapability =
  | 'function_calling'
  | 'streaming'
  | 'vision'
  | 'json_mode'
  | 'structured_output'
  | 'reasoning'
  | 'tools'
  | 'system_prompt'
  | 'web_search'
  | 'code_interpreter'
  | 'caching'
  | 'embeddings'

export type PricingData = {
  success: boolean
  message?: string
  data: PricingModel[]
  vendors: PricingVendor[]
  group_ratio: Record<string, number>
  usable_group: Record<string, { desc: string; ratio: number }>
  supported_endpoint: Record<string, string>
  auto_groups: string[]
}

export type TokenUnit = 'M' | 'K'
export type PriceType =
  | 'input'
  | 'output'
  | 'cache'
  | 'create_cache'
  | 'image'
  | 'audio_input'
  | 'audio_output'
export type QuotaType = 0 | 1 // 0: token-based, 1: per-request
/** One comparison inside a projected tier condition. */
export type BillingDisplayCondition = {
  var: string
  op: string
  value: number
}

/** One projected price branch with per-variable unit prices (USD / 1M tokens). */
export type BillingDisplayTier = {
  label: string
  conditions?: BillingDisplayCondition[]
  condition_text?: string
  condition?: BillingDisplayRule
  unit_prices: Record<string, number>
  constant?: number
  has_constant?: boolean
}

/** One node of the projected conditional-multiplier tree. */
export type BillingDisplayRule = {
  text: string
  multiplier: number
  fallback?: number
  op?: 'and' | 'or' | 'not' | ''
  children?: BillingDisplayRule[]
  source?: 'time' | 'param' | 'header' | 'text' | 'token' | 'usage'
  time_func?: string
  timezone?: string
  compare_op?: string
  value?: string
  path?: string
  text_only?: boolean
}

/**
 * Backend-generated strict read-only projection of a saved billing
 * expression. `exact` means every money structure was explained; `opaque`
 * projections must never be turned into guessed unit prices on the client.
 */
export type BillingDisplayProjection = {
  status: 'exact' | 'opaque'
  reason?: string
  unit: string
  display_version: number
  expression_version: number
  expression_hash: string
  tiers?: BillingDisplayTier[]
  rules?: BillingDisplayRule[]
  constant_charge?: number
  scenarios?: Array<{ matched: boolean; tiers: BillingDisplayTier[] }>
}
