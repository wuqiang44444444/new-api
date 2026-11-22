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
import z from 'zod'

import type { TaskPluginListItem } from '../types'
import {
  countPluginKinds,
  PLUGIN_KIND_OPTIONS,
  PLUGIN_KINDS,
} from './plugin-kinds'
import { resolvePluginParts } from './plugin-parts'

const INSTALLED_PLUGIN_KINDS = [...PLUGIN_KINDS, 'link'] as const
export type InstalledPluginKind = (typeof INSTALLED_PLUGIN_KINDS)[number]

export const INSTALLED_PLUGIN_KIND_OPTIONS = [
  ...PLUGIN_KIND_OPTIONS,
  { value: 'link', label: 'Link extensions' },
] as const

export const taskPluginsSearchSchema = z.object({
  kind: z.enum(INSTALLED_PLUGIN_KINDS).optional().catch(undefined),
})

export function countInstalledPluginKinds(
  plugins: TaskPluginListItem[]
): Record<InstalledPluginKind, number> {
  const native = plugins.filter((plugin) => !plugin.link)
  return {
    ...countPluginKinds(native.map(resolvePluginParts)),
    link: plugins.length - native.length,
  }
}

export function defaultInstalledPluginKind(
  counts: Record<InstalledPluginKind, number>
): InstalledPluginKind {
  return INSTALLED_PLUGIN_KINDS.find((kind) => counts[kind] > 0) ?? 'task'
}

export function matchesInstalledPluginKind(
  plugin: TaskPluginListItem,
  kind: InstalledPluginKind
): boolean {
  if (kind === 'link') return plugin.link === true
  return !plugin.link && resolvePluginParts(plugin)[kind]
}
