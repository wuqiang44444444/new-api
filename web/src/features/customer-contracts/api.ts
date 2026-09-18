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
import type { ApiResponse, UserContractEntities } from '@/features/users/types'
import { api } from '@/lib/api'

import type {
  CustomerContractAdminListResponse,
  CustomerContractMigrationPayload,
  CustomerContractMigrationPreview,
  GetCustomerContractsParams,
} from './types'

export async function getCustomerContracts(
  params: GetCustomerContractsParams
): Promise<CustomerContractAdminListResponse> {
  const res = await api.get('/api/customer-contracts', { params })
  return res.data
}

export async function getCustomerContractMigrationPreview(): Promise<
  ApiResponse<CustomerContractMigrationPreview[]>
> {
  const res = await api.get('/api/customer-contracts/migration/preview')
  return res.data
}

export async function migrateCustomerContract(
  payload: CustomerContractMigrationPayload
): Promise<ApiResponse<UserContractEntities>> {
  const res = await api.post('/api/customer-contracts/migration', payload)
  return res.data
}
