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
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { TaskLog } from '../types'

function DetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-2 text-sm sm:grid-cols-[8rem_minmax(0,1fr)]'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span
        className={cn(
          'min-w-0 text-xs break-all sm:wrap-break-word',
          props.mono && 'font-mono'
        )}
      >
        {props.value}
      </span>
    </div>
  )
}

function DetailSection(props: { label: string; children: React.ReactNode }) {
  return (
    <section className='min-w-0 space-y-1.5'>
      <Label className='flex items-center gap-1.5 text-xs font-semibold'>
        {props.label}
      </Label>
      <div className='bg-muted/30 min-w-0 space-y-1.5 rounded-md border p-2.5'>
        {props.children}
      </div>
    </section>
  )
}

// formatRequestedAudio 区分布尔参数的显式 false 与缺失：缺失显示“未记录”。
function formatRequestedAudio(
  param: { value: boolean } | undefined,
  t: (key: string) => string
): string {
  if (!param) return t('Not recorded')
  return param.value ? 'true' : 'false'
}

export function TaskVideoParameters(props: {
  details: TaskLog['video_details']
}) {
  const { t } = useTranslation()
  const videoDetails = props.details
  return videoDetails ? (
    <DetailSection label={t('Video Parameters')}>
      {videoDetails.request ? (
        <>
          <DetailRow
            label={t('Requested duration')}
            value={videoDetails.request.seconds?.value ?? t('Not recorded')}
            mono
          />
          <DetailRow
            label={t('Requested resolution')}
            value={videoDetails.request.resolution?.value ?? t('Not recorded')}
            mono
          />
          <DetailRow
            label={t('Requested aspect ratio')}
            value={videoDetails.request.ratio?.value ?? t('Not recorded')}
            mono
          />
          <DetailRow
            label={t('Requested generate audio')}
            value={formatRequestedAudio(videoDetails.request.generate_audio, t)}
            mono
          />
          {videoDetails.request.service_tier ? (
            <DetailRow
              label={t('Service tier')}
              value={videoDetails.request.service_tier.value}
              mono
            />
          ) : null}
        </>
      ) : null}
      {videoDetails.billing ? (
        <>
          <DetailRow
            label={t('Billing duration')}
            value={
              videoDetails.billing.duration_seconds?.value ?? t('Not recorded')
            }
            mono
          />
          <DetailRow
            label={t('Billing resolution')}
            value={videoDetails.billing.resolution?.value ?? t('Not recorded')}
            mono
          />
          <DetailRow
            label={t('Billing generate audio')}
            value={formatRequestedAudio(videoDetails.billing.generate_audio, t)}
            mono
          />
        </>
      ) : null}
      {videoDetails.settlement ? (
        <>
          <DetailRow
            label={t('Settled quota')}
            value={formatLogQuota(videoDetails.settlement.quota)}
            mono
          />
          {videoDetails.settlement.billing_state ? (
            <DetailRow
              label={t('Billing state')}
              value={videoDetails.settlement.billing_state}
              mono
            />
          ) : null}
          {videoDetails.settlement.actual_usage_reported &&
          videoDetails.settlement.actual_usage ? (
            <DetailRow
              label={t('Reported usage')}
              value={Object.entries(videoDetails.settlement.actual_usage)
                .map(([key, value]) => `${key}=${value}`)
                .join(', ')}
              mono
            />
          ) : null}
        </>
      ) : null}
    </DetailSection>
  ) : null
}
