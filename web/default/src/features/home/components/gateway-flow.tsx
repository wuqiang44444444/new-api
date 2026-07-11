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
  ArrowDown,
  ChartNoAxesColumnIncreasing,
  CircleCheck,
  RefreshCw,
  Route,
  Shuffle,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface GatewayFlowProps {
  systemName: string
  serverAddress: string
}

const ROUTING_CAPABILITIES = [
  {
    key: 'Multi-channel scheduling',
    description: 'Select the best path from available channels',
    icon: Shuffle,
  },
  {
    key: 'Failure retry',
    description: 'Reduce the impact of single-channel fluctuations',
    icon: RefreshCw,
  },
  {
    key: 'Usage records',
    description: 'Trace requests, tokens, and billing',
    icon: ChartNoAxesColumnIncreasing,
  },
] as const

export function GatewayFlow(props: GatewayFlowProps) {
  const { t } = useTranslation()

  return (
    <div
      className='mx-auto grid w-full max-w-[34rem] min-w-0'
      role='img'
      aria-label={t('Unified API request, routing, and response flow')}
    >
      <article className='home-business-card rounded-lg p-4'>
        <div className='flex items-center gap-2 text-sm font-semibold'>
          <span className='bg-primary size-2 rounded-full' aria-hidden='true' />
          {t('Request')}
        </div>
        <code className='text-muted-foreground mt-2 block text-xs leading-5 [overflow-wrap:anywhere]'>
          POST {props.serverAddress}/v1/chat/completions
          <br />
          Authorization: Bearer sk-your-key
          <br />
          {'{ "model": "<available-model-id>" }'}
        </code>
      </article>

      <ArrowDown
        aria-hidden='true'
        className='text-primary mx-auto my-1 size-5'
      />

      <article className='home-business-card border-primary rounded-lg p-4'>
        <div className='flex items-center gap-2 text-sm font-semibold'>
          <span className='bg-primary text-primary-foreground grid size-7 place-items-center rounded-md'>
            <Sparkles aria-hidden='true' className='size-4' />
          </span>
          {props.systemName} {t('Unified API Gateway')}
        </div>
        <div className='home-business-success mt-3 flex items-center gap-2 text-sm font-semibold'>
          <CircleCheck aria-hidden='true' className='size-4' />
          {t('Routing successful')}
        </div>
        <dl className='text-muted-foreground mt-3 grid gap-2 text-xs'>
          <div className='flex justify-between gap-4'>
            <dt>{t('Model routing')}</dt>
            <dd className='text-foreground font-medium'>
              &lt;available-model-id&gt;
            </dd>
          </div>
          <div className='flex justify-between gap-4'>
            <dt>{t('Processing time')}</dt>
            <dd className='text-foreground font-medium tabular-nums'>1.2s</dd>
          </div>
        </dl>
      </article>

      <ArrowDown
        aria-hidden='true'
        className='text-primary mx-auto my-1 size-5'
      />

      <div className='grid items-center gap-3 sm:grid-cols-[minmax(0,1fr)_11rem]'>
        <article className='home-business-card rounded-lg p-4'>
          <div className='flex items-center gap-2 text-sm font-semibold'>
            <Route aria-hidden='true' className='text-primary size-4' />
            {t('Routing decision')}
          </div>
          <ul className='mt-3 grid gap-2 text-xs'>
            {['A', 'B', 'C'].map((channel, index) => (
              <li
                key={channel}
                className='text-muted-foreground flex items-center justify-between gap-3'
              >
                <span className='flex items-center gap-2'>
                  <span className='bg-accent text-accent-foreground grid size-5 place-items-center rounded-sm font-semibold'>
                    {channel}
                  </span>
                  {t('Available channel {{channel}}', { channel })}
                </span>
                <span className='border-border bg-muted rounded-full border px-2 py-0.5 text-xs'>
                  {t(index === 0 ? 'Primary' : 'Fallback')}
                </span>
              </li>
            ))}
          </ul>
        </article>

        <div className='grid gap-3'>
          {ROUTING_CAPABILITIES.map((capability) => {
            const Icon = capability.icon
            return (
              <div
                key={capability.key}
                className='grid grid-cols-[2rem_1fr] gap-2'
              >
                <span className='bg-accent text-accent-foreground grid size-8 place-items-center rounded-full'>
                  <Icon aria-hidden='true' className='size-4' />
                </span>
                <span>
                  <strong className='text-primary block text-xs font-semibold'>
                    {t(capability.key)}
                  </strong>
                  <span className='text-muted-foreground mt-0.5 block text-xs leading-4'>
                    {t(capability.description)}
                  </span>
                </span>
              </div>
            )
          })}
        </div>
      </div>

      <ArrowDown
        aria-hidden='true'
        className='text-primary mx-auto my-1 size-5'
      />

      <article className='home-business-card rounded-lg p-4'>
        <div className='flex items-center gap-2 text-sm font-semibold'>
          <span className='bg-primary size-2 rounded-full' aria-hidden='true' />
          {t('Response')}
          <span className='home-business-success ml-auto tabular-nums'>
            200 OK
          </span>
        </div>
        <pre className='text-muted-foreground mt-2 overflow-x-auto text-xs leading-5'>{`{
  "id": "response-example",
  "model": "<available-model-id>",
  "usage": { "total_tokens": 27 }
}`}</pre>
      </article>
    </div>
  )
}
