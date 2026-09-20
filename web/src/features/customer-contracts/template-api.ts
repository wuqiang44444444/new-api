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
import type { ApiResponse } from '@/features/users/types'
import { api } from '@/lib/api'

import type {
  ContractTemplateAuditPage,
  ContractTemplateListResponse,
  ContractTemplateOptionsResponse,
  ContractTemplateSnapshotResponse,
  ContractTemplateWritePayload,
  GetContractTemplatesParams,
} from './template-types'

export async function getContractTemplates(
  params: GetContractTemplatesParams
): Promise<ContractTemplateListResponse> {
  const res = await api.get('/api/customer-contract-templates', { params })
  return res.data
}

export async function getContractTemplateOptions(): Promise<ContractTemplateOptionsResponse> {
  const res = await api.get('/api/customer-contract-templates/options')
  return res.data
}

export async function getContractTemplate(
  templateId: number
): Promise<ContractTemplateSnapshotResponse> {
  const res = await api.get(`/api/customer-contract-templates/${templateId}`)
  return res.data
}

export async function createContractTemplate(
  payload: ContractTemplateWritePayload
): Promise<ContractTemplateSnapshotResponse> {
  const res = await api.post('/api/customer-contract-templates', payload)
  return res.data
}

export async function updateContractTemplate(
  templateId: number,
  payload: ContractTemplateWritePayload
): Promise<ContractTemplateSnapshotResponse> {
  const res = await api.put(
    `/api/customer-contract-templates/${templateId}`,
    payload
  )
  return res.data
}

export async function getContractTemplateAudits(
  templateId: number,
  page = 1
): Promise<ApiResponse<ContractTemplateAuditPage>> {
  const res = await api.get(
    `/api/customer-contract-templates/${templateId}/audits`,
    { params: { p: page } }
  )
  return res.data
}
