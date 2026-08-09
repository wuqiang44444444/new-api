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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import {
  AdminBillingReconciliation,
  type AdminBillingSearch,
} from '@/features/billing-reconciliation'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const adminBillingSearchSchema = z.object({
  section: z.enum(['customer', 'upstream']).optional().catch('customer'),
  month: z
    .string()
    .regex(/^\d{4}-\d{2}$/)
    .optional(),
  userId: z.number().positive().optional(),
  dimension: z.enum(['api_key', 'channel']).optional().catch('api_key'),
  customerSearch: z.string().max(100).optional(),
  customerQuality: z
    .enum(['all', 'complete', 'partial'])
    .optional()
    .catch('all'),
  customerSortBy: z
    .enum(['net_quota', 'requests', 'original_quota', 'username'])
    .optional()
    .catch('net_quota'),
  customerSortOrder: z.enum(['asc', 'desc']).optional().catch('desc'),
  customerPage: z.number().int().positive().optional().catch(1),
  customerPageSize: z
    .union([z.literal(10), z.literal(20), z.literal(50), z.literal(100)])
    .optional()
    .catch(20),
})

export const Route = createFileRoute('/_authenticated/admin/billing/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: adminBillingSearchSchema,
  component: AdminBillingRoute,
})

function AdminBillingRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()

  const handleSearchChange = (
    patch: Partial<AdminBillingSearch>,
    replace = false
  ) =>
    navigate({
      search: (current) => ({ ...current, ...patch }),
      replace,
    })

  return (
    <AdminBillingReconciliation
      search={search}
      onSearchChange={handleSearchChange}
    />
  )
}
