import type { UseFormReturn } from 'react-hook-form'

import { CHANNEL_TYPE_MINIMAX_LINK } from '../constants'
import type { Channel } from '../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  type ChannelFormValues,
} from './channel-form'
import { parseChannelOtherSettings } from './channel-utils'

export const MINIMAX_NATIVE_TYPE = 35
export const MINIMAX_FILTER = 'minimax'

export function isMinimaxChannel(type: number): boolean {
  return type === MINIMAX_NATIVE_TYPE || type === CHANNEL_TYPE_MINIMAX_LINK
}

export function channelManagementTypeOptions<T extends { value: number }>(
  options: readonly T[],
  editing: boolean,
  currentType: number
): T[] {
  return options.filter(
    (option) =>
      option.value !== CHANNEL_TYPE_MINIMAX_LINK &&
      (!editing ||
        isMinimaxChannel(currentType) ||
        option.value !== MINIMAX_NATIVE_TYPE)
  )
}

export function minimaxAccessLabel(
  channel: Pick<Channel, 'type' | 'settings'>
): string {
  if (channel.type === MINIMAX_NATIVE_TYPE) return 'Native API'
  return parseChannelOtherSettings(channel.settings)
    ?.video_upstream_protocol === 'jdcloud_video_task_v1'
    ? 'JD Cloud · Standard video'
    : 'Unrecognized video protocol'
}

// Only UI grouping lives here. Channel configuration and runtime identities
// continue to use the original numeric types.
export function channelManagementFilter(value?: string): {
  type?: number
  types?: string
} {
  if (value === MINIMAX_FILTER) return { types: '35,64' }
  if (!value || value === 'all') return {}
  return { type: Number(value) }
}

export function channelManagementFilterLabel(
  type: number,
  fallback: string
): string {
  if (type === MINIMAX_NATIVE_TYPE) return 'MiniMax · Native API'
  if (type === CHANNEL_TYPE_MINIMAX_LINK) return 'MiniMax · Standard video'
  return fallback
}

// A mode change is a new connection draft. Keep only provider-independent
// identity fields, preventing credentials and private settings crossing modes.
export function resetMinimaxConnectionDraft(
  form: UseFormReturn<ChannelFormValues>,
  type: number,
  accessSelected: boolean
) {
  const current = form.getValues()
  form.reset(
    {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: current.name,
      remark: current.remark,
      tag: current.tag,
      group: current.group,
      status: current.status,
      type,
      video_upstream_protocol:
        type === CHANNEL_TYPE_MINIMAX_LINK
          ? 'jdcloud_video_task_v1'
          : CHANNEL_FORM_DEFAULT_VALUES.video_upstream_protocol,
      asset_upstream_protocol:
        type === CHANNEL_TYPE_MINIMAX_LINK
          ? 'none'
          : CHANNEL_FORM_DEFAULT_VALUES.asset_upstream_protocol,
      minimax_plugin_version: '',
      seedance_plugin_version: '',
      minimax_access_selected: accessSelected,
    },
    { keepDefaultValues: true }
  )
}

export function selectChannelManagementType(
  form: UseFormReturn<ChannelFormValues>,
  type: number,
  editing: boolean
) {
  if (!Number.isInteger(type) || type <= 0) return
  const previous = form.getValues('type')
  if (editing && (isMinimaxChannel(previous) || isMinimaxChannel(type))) return
  if (isMinimaxChannel(type)) {
    resetMinimaxConnectionDraft(form, MINIMAX_NATIVE_TYPE, false)
  } else if (isMinimaxChannel(previous)) {
    resetMinimaxConnectionDraft(form, type, true)
  } else {
    form.setValue('type', type, { shouldDirty: true, shouldValidate: true })
  }
}
