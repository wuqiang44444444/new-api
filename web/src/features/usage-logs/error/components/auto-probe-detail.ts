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
import type { ErrorLogItem } from '../api'

export function mediaAutoProbeDetail(
  entry: ErrorLogItem
): Record<string, string> | null {
  if (entry.event_type !== 'channel_test') return null
  try {
    const detail: unknown = JSON.parse(entry.detail || '{}')
    if (!detail || typeof detail !== 'object' || Array.isArray(detail)) {
      return null
    }
    const fields = detail as Record<string, unknown>
    if (fields.test_mode !== 'auto') return null
    // Old media observations have no probe_media marker. Only use existing
    // known affected model families for their labels; never relabel text errors.
    const model = entry.model_name.toLowerCase()
    let media: string
    if (fields.probe_media === 'image' || fields.probe_media === 'video') {
      media = fields.probe_media
    } else {
      if (model.startsWith('gpt-image-')) media = 'image'
      else if (model.startsWith('seedance')) media = 'video'
      else return null
    }
    const strings = Object.fromEntries(
      Object.entries(fields).filter(
        (item): item is [string, string] => typeof item[1] === 'string'
      )
    )
    return { ...strings, probe_media: media }
  } catch {
    return null
  }
}
