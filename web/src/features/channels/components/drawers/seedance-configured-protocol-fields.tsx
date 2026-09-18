import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'

import type { ChannelFormValues } from '../../lib/channel-form'
import {
  getSeedancePluginConfiguration,
  type SeedanceConfigurationSnapshot,
} from '../../lib/seedance-plugin-configuration'
import {
  SeedanceProtocolFields,
  type SeedanceProtocolFieldsProps,
} from './seedance-protocol-fields'

export type SeedanceConfiguredProtocolFieldsProps = Omit<
  SeedanceProtocolFieldsProps,
  'configuration'
>

export function SeedanceConfiguredProtocolFields(
  props: SeedanceConfiguredProtocolFieldsProps
) {
  const formId = useId()
  const form = useFormContext<ChannelFormValues>()
  const [snapshot, setSnapshot] = useState<SeedanceConfigurationSnapshot>()
  const formVersion = useWatch({
    control: form.control,
    name: 'seedance_plugin_version',
  })
  // Each opened form keeps its own declaration. Background cache refreshes
  // must not change the version attached to values the administrator is editing.
  const configuration = useQuery({
    queryKey: ['seedance-channel-configuration', formId],
    queryFn: getSeedancePluginConfiguration,
    enabled: snapshot === undefined,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const setValue = form.setValue
  useEffect(() => {
    if (configuration.data && !snapshot) {
      setSnapshot(configuration.data)
    }
    if (snapshot && formVersion !== snapshot.version) {
      setValue(
        'seedance_plugin_configuration',
        snapshot.configuration ?? undefined
      )
      setValue('seedance_plugin_version', snapshot.version)
    }
  }, [configuration.data, snapshot, formVersion, setValue])

  if (!snapshot && configuration.isError) {
    return <ErrorState onRetry={() => void configuration.refetch()} />
  }
  if (!snapshot) return <LoadingState />
  if (!snapshot.configuration)
    return (
      <ErrorState
        onRetry={() => {
          setSnapshot(undefined)
          void configuration.refetch()
        }}
      />
    )
  return (
    <SeedanceProtocolFields {...props} configuration={snapshot.configuration} />
  )
}
