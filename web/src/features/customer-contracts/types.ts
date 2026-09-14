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
export type CustomerContractAdminStatus = 'active' | 'inactive'

export interface CustomerContractAdminListItem {
  contract_id: number
  contract_name: string
  user_id: number
  username: string
  display_name: string
  contract_enabled: boolean
  contract_status: CustomerContractAdminStatus
  contract_version: number
  rule_count: number
  unavailable_rule_count: number
  bound_token_count: number
  updated_at: number
  admin_user_id: number
  admin_username: string
}

export interface CustomerContractAdminSummary {
  total: number
  active: number
  inactive: number
}

export interface CustomerContractAdminListPage {
  page: number
  page_size: number
  total: number
  items: CustomerContractAdminListItem[]
  summary: CustomerContractAdminSummary
}

export interface CustomerContractAdminListResponse {
  success: boolean
  message?: string
  data?: CustomerContractAdminListPage
}

export interface GetCustomerContractsParams {
  p?: number
  page_size?: number
  keyword?: string
  status?: CustomerContractAdminStatus | ''
}

export interface CustomerContractsSearch {
  page?: number
  pageSize?: number
  filter?: string
  status?: CustomerContractAdminStatus[]
}

export const EMPTY_CUSTOMER_CONTRACT_SUMMARY: CustomerContractAdminSummary = {
  total: 0,
  active: 0,
  inactive: 0,
}

export interface CustomerContractMigrationRulePreview {
  public_model: string
  route_group: string
  ratio_units: number
  channel_ids: number[]
  resolved_channel_id: number
  needs_decision: boolean
}

export interface CustomerContractMigrationPreview {
  user_id: number
  username: string
  contract_enabled: boolean
  bound_token_count: number
  already_migrated: boolean
  rules: CustomerContractMigrationRulePreview[]
}

export interface CustomerContractMigrationPayload {
  user_id: number
  contract_name: string
  reason: string
  channel_overrides: Record<string, number>
}
