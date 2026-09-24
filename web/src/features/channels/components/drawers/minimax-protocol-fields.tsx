import { useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState } from 'react'
import { useFormContext } from 'react-hook-form'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { useTranslation } from 'react-i18next'

import type { ChannelFormValues } from '../../lib/channel-form'
import {
  getMinimaxPluginConfiguration,
  type MinimaxConfigurationSnapshot,
} from '../../lib/minimax-plugin-configuration'

// MinimaxProtocolFields pins the declaration version the form was rendered
// from and displays the fixed protocol facts. The JD Cloud task protocol is
// code-registered: administrators configure connection, models and mapping,
// never protocol JSON.
export function MinimaxProtocolFields() {
  const { t } = useTranslation()
  const formId = useId()
  const form = useFormContext<ChannelFormValues>()
  const [snapshot, setSnapshot] = useState<MinimaxConfigurationSnapshot>()
  const configuration = useQuery({
    queryKey: ['minimax-channel-configuration', formId],
    queryFn: getMinimaxPluginConfiguration,
    enabled: snapshot === undefined,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  useEffect(() => {
    if (configuration.data && !snapshot) {
      setSnapshot(configuration.data)
      form.setValue('minimax_plugin_version', configuration.data.version)
    }
  }, [configuration.data, snapshot, form])
  if (!snapshot && configuration.isError) {
    return (
      <ErrorState
        onRetry={() => {
          setSnapshot(undefined)
          void configuration.refetch()
        }}
      />
    )
  }
  if (!snapshot) return <LoadingState />
  return (
    <FormField
      control={form.control}
      name='minimax_plugin_version'
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('Plugin declaration version')}</FormLabel>
          <FormControl>
            <Input {...field} readOnly />
          </FormControl>
          <FormDescription>
            {t(
              'The video protocol is fixed by code. First-phase open set: single text prompt, 6 seconds, 768p, 16:9, no watermark; the platform always enables prompt optimization.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
