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
import { api } from '@/lib/api'

export type ObjectStorageBackend = 'upstream' | 's3' | 'azure_blob'

export type ObjectStorageSettingView = {
  backend: ObjectStorageBackend | ''
  endpoint: string
  bucket: string
  prefix: string
  region: string
  account_name: string
  account_name_masked: string
  credential_configured: boolean
  revision: string
  last_test_status: string
  last_test_at: number
  active: boolean
  env_import_available: boolean
}

export type ObjectStorageTestStep = {
  name: string
  success: boolean
  detail?: string
}

export type ObjectStorageTestResult = {
  success: boolean
  message?: string
  steps?: ObjectStorageTestStep[]
  cleanup_failed: boolean
}

export type ObjectStorageSettingRequest = {
  backend: ObjectStorageBackend
  endpoint: string
  bucket: string
  prefix: string
  region: string
  account_name: string
  credential: string
  input_mode: 'connection_string' | 'manual'
  connection_string: string
}

export type ObjectStorageEnvImportPreview = {
  backend: string
  endpoint: string
  bucket: string
  prefix: string
  region: string
  account_name_masked: string
  credential_configured: boolean
}

type ObjectStorageApiResponse<T> = {
  success: boolean
  message: string
  data?: T
}

export async function getObjectStorageSetting() {
  const res =
    await api.get<ObjectStorageApiResponse<ObjectStorageSettingView>>(
      '/api/option/object_storage'
    )
  return res.data
}

export async function testObjectStorageConnection(
  request: ObjectStorageSettingRequest
) {
  const res = await api.post<
    ObjectStorageApiResponse<ObjectStorageTestResult>
  >('/api/option/object_storage/test', request)
  return res.data
}

export async function saveObjectStorageSetting(
  request: ObjectStorageSettingRequest
) {
  const res = await api.put<
    ObjectStorageApiResponse<ObjectStorageTestResult | undefined>
  >('/api/option/object_storage', request)
  return res.data
}

export async function previewObjectStorageEnvImport() {
  const res = await api.post<
    ObjectStorageApiResponse<ObjectStorageEnvImportPreview | undefined>
  >('/api/option/object_storage/import_preview', {})
  return res.data
}

export async function importObjectStorageEnvConfig() {
  const res = await api.post<
    ObjectStorageApiResponse<ObjectStorageTestResult | undefined>
  >('/api/option/object_storage/import', {})
  return res.data
}
