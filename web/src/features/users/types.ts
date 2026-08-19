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
import { z } from 'zod'

import type { AdminPermissionMatrix } from '@/lib/admin-permissions'

// ============================================================================
// User Schema & Types
// ============================================================================

/** User status: 1 = enabled, 2 = disabled, 3+ = other states */
export const userStatusSchema = z.number()
export type UserStatus = z.infer<typeof userStatusSchema>

/** User role: 1 = common user, 10 = admin, 100 = root */
export const userRoleSchema = z.number()
export type UserRole = z.infer<typeof userRoleSchema>

export const userSchema = z.object({
  id: z.number(),
  username: z.string(),
  display_name: z.string(),
  password: z.string().optional(),
  github_id: z.string().optional(),
  oidc_id: z.string().optional(),
  wechat_id: z.string().optional(),
  telegram_id: z.string().optional(),
  email: z.string().optional(),
  quota: z.number(),
  used_quota: z.number(),
  request_count: z.number(),
  group: z.string(),
  aff_code: z.string().optional(),
  aff_count: z.number().optional(),
  aff_quota: z.number().optional(),
  aff_history_quota: z.number().optional(),
  inviter_id: z.number().optional(),
  linux_do_id: z.string().optional(),
  status: userStatusSchema,
  role: userRoleSchema,
  created_at: z.number().optional(),
  updated_at: z.number().optional(),
  last_login_at: z.number().optional(),
  DeletedAt: z.any().nullable().optional(),
  remark: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
  contract_mode: z.boolean().optional().default(false),
  contract_version: z.number().optional().default(0),
  contract_rule_count: z.number().optional().default(0),
})
export type User = z.infer<typeof userSchema>

export const userListSchema = z.array(userSchema)

// ============================================================================
// API Request/Response Types
// ============================================================================

/** Generic API response */
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface CustomerContractPricePreview {
  price_type: 'model_price' | 'model_ratio' | 'tiered_multiplier'
  billing_mode?: 'per_call' | 'per_token' | 'tiered_expr' | string
  base_model_price?: string
  final_model_price?: string
  base_model_ratio?: string
  final_model_ratio?: string
  completion_ratio?: string
  base_image_ratio?: string
  final_image_ratio?: string
  current_discounted_price?: string
}

export interface CustomerContractRule {
  model: string
  route_group: string
  discount: string
  available: boolean
  native_group_ratio: string
  effective_multiplier: string
  special_group_ratio: boolean
  price: CustomerContractPricePreview
}

export interface CustomerContract {
  user_id: number
  username: string
  contract_mode: boolean
  contract_version: number
  rules: CustomerContractRule[]
  disable_warning?: string
}

export interface CustomerContractGroupOption {
  group: string
  models: string[]
  prices: Record<string, CustomerContractPricePreview>
  native_group_ratio: string
  special_group_ratio: boolean
}

export interface CustomerContractAudit {
  id: number
  contract_version: number
  admin_user_id: number
  admin_username: string
  operation: 'create' | 'update' | 'enable' | 'disable'
  reason: string
  before_rule_count: number
  after_rule_count: number
  created_at: number
}

export interface CustomerContractAuditPage {
  items: CustomerContractAudit[]
  total: number
  page: number
  page_size: number
}

export interface CustomerContractWritePayload {
  expected_version: number
  enabled: boolean
  reason: string
  rules: Array<{ model: string; route_group: string; discount: string }>
}

export type UserSortBy =
  | 'id'
  | 'username'
  | 'quota'
  | 'group'
  | 'created_at'
  | 'last_login_at'

export type UserSortOrder = 'asc' | 'desc'

export interface GetUsersParams {
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface GetUsersResponse {
  success: boolean
  message?: string
  data?: {
    items: User[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchUsersParams {
  keyword?: string
  group?: string
  role?: string
  status?: string
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface UserFormData {
  username: string
  email?: string
  display_name: string
  password?: string
  role?: number // Only used when creating user
  quota?: number // Only used when updating user
  group?: string // Only used when updating user
  remark?: string // Only used when updating user
  admin_permissions?: AdminPermissionMatrix
}

export type ManageUserAction =
  | 'promote'
  | 'demote'
  | 'enable'
  | 'disable'
  | 'delete'
  | 'add_quota'

export type QuotaAdjustMode = 'add' | 'subtract' | 'override'

export interface ManageUserQuotaPayload {
  id: number
  action: 'add_quota'
  mode: QuotaAdjustMode
  value: number
}

// ============================================================================
// Dialog Types
// ============================================================================

export type UsersDialogType = 'create' | 'update' | 'delete'
