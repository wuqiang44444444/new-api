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
import type {
  ApiResponse,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
  CustomerContractRulePayload,
} from '@/features/users/types'

export interface ContractTemplateListItem {
  id: number
  name: string
  enabled: boolean
  version: number
  model_count: number
  rule_count: number
  stale_rule_count: number
  creator_id: number
  updater_id: number
  updater_name: string
  created_at: number
  updated_at: number
}

export interface ContractTemplateListPage {
  items: ContractTemplateListItem[]
  total: number
  page: number
  page_size: number
}

export interface GetContractTemplatesParams {
  p?: number
  page_size?: number
  keyword?: string
  enabled?: boolean | ''
}

export interface ContractTemplateListResponse {
  success: boolean
  message?: string
  data?: ContractTemplateListPage
}

export interface ContractTemplateRuleView {
  public_model: string
  channel_id: number
  route_group: string
  ratio_units: number
  available: boolean
}

export interface ContractTemplateSnapshot {
  id: number
  name: string
  enabled: boolean
  version: number
  creator_id: number
  updater_id: number
  rules: ContractTemplateRuleView[]
}

export interface ContractTemplateWritePayload {
  expected_version?: number
  enabled?: boolean
  name: string
  reason: string
  rules: CustomerContractRulePayload[]
}

export interface ContractTemplateOptionSources {
  options: CustomerContractGroupOption[]
  channels: CustomerContractChannelGroupOption[]
  customer_context: boolean
}

export interface ContractTemplateAudit {
  id: number
  template_id: number
  template_version: number
  admin_user_id: number
  admin_username: string
  operation: 'create' | 'update' | 'enable' | 'disable'
  reason: string
  before_enabled: boolean
  after_enabled: boolean
  before_rule_count: number
  after_rule_count: number
  created_at: number
}

export interface ContractTemplateAuditPage {
  items: ContractTemplateAudit[]
  total: number
  page: number
  page_size: number
}

export type ContractTemplateSnapshotResponse =
  ApiResponse<ContractTemplateSnapshot>

export type ContractTemplateOptionsResponse =
  ApiResponse<ContractTemplateOptionSources>
