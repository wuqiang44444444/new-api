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
import {
  ChartNoAxesCombined,
  Code2,
  Network,
  ReceiptText,
  Route,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TokenAiMark } from '@/assets/token-ai-mark'
import { cn } from '@/lib/utils'

type AuthGatewayPreviewProps = {
  systemName: string
}

type GatewayStageProps = {
  icon: LucideIcon
  label: string
  emphasis?: boolean
  brand?: boolean
}

const CAPABILITIES = [
  { key: 'Resilient multi-channel routing', icon: Route },
  { key: 'Transparent billing and logs', icon: ReceiptText },
  { key: 'Observability', icon: ChartNoAxesCombined },
] as const

function GatewayStage(props: GatewayStageProps) {
  const Icon = props.icon

  return (
    <li
      className={cn(
        'bg-background flex min-h-28 min-w-0 flex-1 flex-col items-center justify-center gap-3 rounded-lg border px-3 text-center',
        props.emphasis && 'border-primary'
      )}
    >
      <span
        className={cn(
          'text-primary bg-primary/5 flex size-11 items-center justify-center rounded-lg',
          props.emphasis && 'bg-primary/10'
        )}
      >
        {props.brand ? (
          <TokenAiMark className='size-7' />
        ) : (
          <Icon aria-hidden='true' className='size-6' />
        )}
      </span>
      <span className='text-sm font-medium'>{props.label}</span>
    </li>
  )
}

export function AuthGatewayPreview(props: AuthGatewayPreviewProps) {
  const { t } = useTranslation()

  return (
    <div className='mx-auto w-full max-w-2xl'>
      <div className='max-w-xl'>
        <p className='text-primary text-sm font-medium'>
          {t('Professional Token Service & Enterprise AI Gateway')}
        </p>
        <h2 className='mt-4 text-2xl font-semibold tracking-tight'>
          {t('Reliable AI model access.')}
        </h2>
        <p className='text-muted-foreground mt-3 text-base leading-7'>
          {t('One API for leading AI models')}
        </p>
      </div>

      <ol
        className='mt-12 flex items-center'
        aria-label={t('Unified API request, routing, and response flow')}
      >
        <GatewayStage icon={Code2} label={t('Application')} />
        <li
          aria-hidden='true'
          className='border-border min-w-6 flex-1 border-t border-dashed'
        />
        <GatewayStage
          icon={Network}
          label={`${props.systemName} API`}
          emphasis
          brand
        />
        <li
          aria-hidden='true'
          className='border-border min-w-6 flex-1 border-t border-dashed'
        />
        <GatewayStage icon={Sparkles} label={t('AI models')} />
      </ol>

      <ul className='mt-12 grid grid-cols-3 gap-5'>
        {CAPABILITIES.map((capability) => {
          const Icon = capability.icon

          return (
            <li
              key={capability.key}
              className='text-muted-foreground flex min-w-0 items-center gap-2 text-sm'
            >
              <Icon
                aria-hidden='true'
                className='text-primary size-5 shrink-0'
              />
              <span>{t(capability.key)}</span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
