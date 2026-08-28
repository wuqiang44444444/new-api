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
import type { Channel } from '../types'
import type { ChannelFormValues } from './channel-form'

export type AssetTenantBoundaryField =
  | 'base_url'
  | 'video_upstream_protocol'
  | 'asset_upstream_protocol'
  | 'asset_provider_project'
  | 'asset_region'

export type AssetTenantBoundarySnapshot = Record<
  AssetTenantBoundaryField,
  string
>

export type AssetTenantBoundaryChange = {
  field: AssetTenantBoundaryField
  previous: string
  next: string
}

export const ASSET_TENANT_BOUNDARY_FIELD_LABELS: Record<
  AssetTenantBoundaryField,
  string
> = {
  base_url: 'Base URL',
  video_upstream_protocol: 'Seedance Video Protocol',
  asset_upstream_protocol: 'Seedance Asset Protocol',
  asset_provider_project: 'Provider Project',
  asset_region: 'Provider Region',
}

const BOUNDARY_FIELDS: AssetTenantBoundaryField[] = [
  'base_url',
  'video_upstream_protocol',
  'asset_upstream_protocol',
  'asset_provider_project',
  'asset_region',
]

function normalizeBoundaryValue(
  field: AssetTenantBoundaryField,
  value: unknown
): string {
  const normalized = typeof value === 'string' ? value.trim() : ''
  return field === 'base_url' ? normalized.replace(/\/+$/, '') : normalized
}

function parseSettings(settings: unknown): Record<string, unknown> {
  if (typeof settings !== 'string' || !settings.trim()) return {}
  try {
    const parsed: unknown = JSON.parse(settings)
    return typeof parsed === 'object' && parsed !== null
      ? (parsed as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

export function assetTenantBoundarySnapshotFromChannel(
  channel?: Channel | null
): AssetTenantBoundarySnapshot | null {
  if (!channel) return null
  const settings = parseSettings(channel.settings)
  return {
    base_url: normalizeBoundaryValue('base_url', channel.base_url),
    video_upstream_protocol: normalizeBoundaryValue(
      'video_upstream_protocol',
      settings.video_upstream_protocol || 'modelark_v3_volcengine'
    ),
    asset_upstream_protocol: normalizeBoundaryValue(
      'asset_upstream_protocol',
      settings.asset_upstream_protocol || 'none'
    ),
    asset_provider_project: normalizeBoundaryValue(
      'asset_provider_project',
      settings.asset_provider_project
    ),
    asset_region: normalizeBoundaryValue('asset_region', settings.asset_region),
  }
}

export function assetTenantBoundarySnapshotFromForm(
  formData: Pick<
    ChannelFormValues,
    | 'base_url'
    | 'video_upstream_protocol'
    | 'asset_upstream_protocol'
    | 'asset_provider_project'
    | 'asset_region'
  >
): AssetTenantBoundarySnapshot {
  return {
    base_url: normalizeBoundaryValue('base_url', formData.base_url),
    video_upstream_protocol: normalizeBoundaryValue(
      'video_upstream_protocol',
      formData.video_upstream_protocol || 'modelark_v3_volcengine'
    ),
    asset_upstream_protocol: normalizeBoundaryValue(
      'asset_upstream_protocol',
      formData.asset_upstream_protocol || 'none'
    ),
    asset_provider_project: normalizeBoundaryValue(
      'asset_provider_project',
      formData.asset_provider_project
    ),
    asset_region: normalizeBoundaryValue('asset_region', formData.asset_region),
  }
}

export function collectAssetTenantBoundaryChanges(
  previous: AssetTenantBoundarySnapshot | null,
  next: AssetTenantBoundarySnapshot
): AssetTenantBoundaryChange[] {
  if (!previous || previous.asset_upstream_protocol === 'none') return []
  return BOUNDARY_FIELDS.flatMap((field) =>
    previous[field] === next[field]
      ? []
      : [{ field, previous: previous[field], next: next[field] }]
  )
}
