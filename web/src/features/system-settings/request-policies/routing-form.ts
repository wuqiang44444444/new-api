import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

function isChannelID(raw: string) {
  const value = raw.trim()
  return /^[1-9][0-9]*$/.test(value) && Number.isSafeInteger(Number(value))
}

export function createRoutingPolicySchema(t: TFunction) {
  return z.object({
    RetryTimes: z.number().int().min(0).max(99),
    AutomaticRetryStatusCodes: z
      .string()
      .refine(
        (value) => parseHttpStatusCodeRules(value).ok,
        t('Invalid status code rules')
      ),
    channel_affinity_setting: z
      .object({
        enabled: z.boolean(),
        session_mode: z.enum(['', 'off', 'prefer', 'strict']),
        switch_on_success: z.boolean(),
        keep_on_channel_disabled: z.boolean(),
        max_entries: z.number().int().min(0),
        default_ttl_seconds: z.number().int().min(0),
        failover_mode: z.enum(['off', 'observe', 'enabled']),
        failover_channel_ids: z
          .string()
          .refine(
            (value) => !value.trim() || value.split(',').every(isChannelID),
            t('Enter comma-separated positive channel IDs.')
          ),
        layered_affinity: z.boolean(),
        failover_verified_pairs: z.string().refine((value) => {
          try {
            return Array.isArray(JSON.parse(value))
          } catch {
            return false
          }
        }, t('Verified pairs must be a JSON array')),
        rules: z.string().superRefine((value, context) => {
          try {
            const rules: unknown = JSON.parse(value)
            if (!Array.isArray(rules)) {
              context.addIssue({
                code: 'custom',
                message: t('Rules JSON must be an array'),
              })
            }
          } catch {
            context.addIssue({
              code: 'custom',
              message: t('Invalid rules JSON format'),
            })
          }
        }),
      })
      .refine(
        (value) =>
          value.failover_mode !== 'enabled' ||
          !!value.failover_channel_ids.trim(),
        {
          path: ['failover_channel_ids'],
          message: t('Choose rollout channels before enabling failover.'),
        }
      ),
  })
}

export type RoutingPolicyFormValues = z.infer<
  ReturnType<typeof createRoutingPolicySchema>
>

export function routingPolicyFormValues(
  options: Record<string, string>
): RoutingPolicyFormValues {
  return {
    RetryTimes: Number(options.RetryTimes),
    AutomaticRetryStatusCodes: options.AutomaticRetryStatusCodes,
    channel_affinity_setting: {
      enabled: options['channel_affinity_setting.enabled'] === 'true',
      session_mode: (options['channel_affinity_setting.session_mode'] ||
        '') as RoutingPolicyFormValues['channel_affinity_setting']['session_mode'],
      switch_on_success:
        options['channel_affinity_setting.switch_on_success'] === 'true',
      keep_on_channel_disabled:
        options['channel_affinity_setting.keep_on_channel_disabled'] === 'true',
      max_entries: Number(options['channel_affinity_setting.max_entries']),
      default_ttl_seconds: Number(
        options['channel_affinity_setting.default_ttl_seconds']
      ),
      rules: options['channel_affinity_setting.rules'] || '[]',
      failover_mode: parseFailoverMode(
        options['channel_affinity_setting.failover_mode']
      ),
      failover_channel_ids: parseRolloutChannels(
        options['channel_affinity_setting.failover_channel_ids']
      ),
      layered_affinity:
        options['channel_affinity_setting.layered_affinity'] === 'true',
      failover_verified_pairs: parseVerifiedPairs(
        options['channel_affinity_setting.failover_verified_pairs']
      ),
    },
  }
}

function parseFailoverMode(value: string | undefined) {
  if (value === 'enabled' || value === 'observe') return value
  return 'off'
}

function parseVerifiedPairs(raw: string | undefined) {
  return raw && raw !== 'null' ? raw : '[]'
}

function parseRolloutChannels(raw: string | undefined) {
  try {
    const ids: unknown = JSON.parse(raw || '[]')
    return Array.isArray(ids) ? ids.join(', ') : ''
  } catch {
    return ''
  }
}

export function routingPolicyOptions(
  values: RoutingPolicyFormValues
): Record<string, string> {
  return {
    RetryTimes: String(values.RetryTimes),
    AutomaticRetryStatusCodes: values.AutomaticRetryStatusCodes,
    ...Object.fromEntries(
      Object.entries(values.channel_affinity_setting).map(([key, value]) => [
        `channel_affinity_setting.${key}`,
        key === 'failover_channel_ids'
          ? JSON.stringify([
              ...new Set(String(value).split(',').map(Number).filter(Boolean)),
            ])
          : String(value),
      ])
    ),
  }
}
