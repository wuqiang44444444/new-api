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
import { useCallback } from 'react'
import { z } from 'zod'

import {
  CustomerContracts,
  type CustomerContractsSearch,
} from '@/features/customer-contracts'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const customerContractsSearchSchema = z.object({
  page: z.number().int().positive().optional().catch(1),
  pageSize: z.number().int().positive().max(100).optional().catch(undefined),
  filter: z.string().max(255).optional().catch(''),
  status: z
    .array(z.enum(['active', 'zero_access', 'inactive']))
    .optional()
    .catch([]),
})

export const Route = createFileRoute(
  '/_authenticated/admin/customer-contracts/'
)({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: customerContractsSearchSchema,
  component: CustomerContractsRoute,
})

function CustomerContractsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()

  const handleSearchChange = useCallback(
    (patch: Partial<CustomerContractsSearch>, replace = false) => {
      void navigate({
        search: (current) => ({ ...current, ...patch }),
        replace,
      })
    },
    [navigate]
  )

  return (
    <CustomerContracts search={search} onSearchChange={handleSearchChange} />
  )
}
