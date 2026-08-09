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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { type Control, useWatch } from 'react-hook-form'

import { getLinkImplementations } from '../../api'
import type { ChannelFormValues } from '../../lib/channel-form'
import {
  linkAccessPlanProviderModelDefaults,
  linkAccessPlansForChannelType,
} from '../../lib/link-access-plan'
import { ModelMappingEditor } from '../model-mapping-editor'

type LinkAwareModelMappingEditorProps = {
  control: Control<ChannelFormValues>
  channelType: number
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

export function LinkAwareModelMappingEditor(
  props: LinkAwareModelMappingEditorProps
) {
  const selectedID =
    useWatch({ control: props.control, name: 'link_implementation_id' }) || ''
  const selectedVersion =
    useWatch({
      control: props.control,
      name: 'link_implementation_version',
    }) || ''
  const { data } = useQuery({
    queryKey: ['link_implementations'],
    queryFn: getLinkImplementations,
  })
  const targetModelOptions = useMemo(() => {
    const implementation = linkAccessPlansForChannelType(
      data?.data || [],
      props.channelType
    ).find(
      (candidate) =>
        candidate.id === selectedID && candidate.version === selectedVersion
    )
    if (implementation) {
      return linkAccessPlanProviderModelDefaults(implementation)
    }
    if (selectedID || selectedVersion) {
      return []
    }

    return props.targetModelOptions || []
  }, [
    data?.data,
    props.channelType,
    props.targetModelOptions,
    selectedID,
    selectedVersion,
  ])

  return (
    <ModelMappingEditor
      value={props.value}
      onChange={props.onChange}
      disabled={props.disabled}
      sourceModelOptions={props.sourceModelOptions}
      targetModelOptions={targetModelOptions}
    />
  )
}
