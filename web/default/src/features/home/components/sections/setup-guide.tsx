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
import { CheckCircle2, ChevronRight, Copy, Database } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'

import { resolveHomeBaseUrl } from '../../lib/server-address'
import {
  buildSetupCode,
  SETUP_SCENES,
  SETUP_TOOLS,
  type SetupSceneId,
} from '../setup-guide-data'

const PREPARATION_STEPS = [
  {
    title: 'Register or sign in',
    description: 'Open the platform console',
  },
  {
    title: 'Create API Key',
    description: 'Examples only use the sk-your-key placeholder',
  },
  {
    title: 'Select an available model',
    description: 'Copy a model ID from your account model list',
  },
] as const

export function SetupGuide() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [sceneId, setSceneId] = useState<SetupSceneId>('editor')
  const [activeToolId, setActiveToolId] = useState('claude-code')
  const tools = SETUP_TOOLS.filter((tool) => tool.scene === sceneId)
  const activeTool = tools.find((tool) => tool.id === activeToolId) ?? tools[0]
  const baseUrl = resolveHomeBaseUrl(status)
  const protocolBaseUrl =
    activeTool.protocol === 'openai' ? `${baseUrl}/v1` : baseUrl
  const setupCode = buildSetupCode(activeTool, baseUrl)

  const copyValue = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(t('Copied to clipboard'))
    } catch {
      toast.error(t('Copy failed'))
    }
  }

  const selectScene = (nextSceneId: SetupSceneId) => {
    setSceneId(nextSceneId)
    const firstTool = SETUP_TOOLS.find((tool) => tool.scene === nextSceneId)
    if (firstTool) setActiveToolId(firstTool.id)
  }

  return (
    <section
      id='configuration'
      className='bg-muted border-border border-y px-4 py-20 sm:px-6 sm:py-24'
      aria-labelledby='configuration-title'
    >
      <div className='mx-auto max-w-6xl text-center'>
        <p className='text-primary text-sm font-semibold'>
          {t('Unified access, flexible tools')}
        </p>
        <h2
          id='configuration-title'
          className='mt-3 text-[clamp(1.75rem,3vw,2.375rem)] leading-tight font-semibold tracking-[-0.035em]'
        >
          {t('One API for leading AI models')}
        </h2>
        <p className='text-muted-foreground mx-auto mt-4 max-w-3xl text-base leading-7'>
          {t(
            'Prepare an API Key, the current site Base URL, and an available model ID, then follow the instructions for your preferred tool.'
          )}
        </p>

        <div
          className='bg-card border-border mt-8 grid overflow-hidden rounded-lg border text-left sm:grid-cols-3'
          aria-label={t('Before configuration')}
        >
          {PREPARATION_STEPS.map((step, index) => (
            <div
              key={step.title}
              className='border-border grid grid-cols-[1.75rem_1fr] gap-3 border-t p-4 first:border-t-0 sm:border-t-0 sm:border-l sm:first:border-l-0'
            >
              <span className='bg-accent text-accent-foreground grid size-7 place-items-center rounded-full text-xs font-semibold tabular-nums'>
                {index + 1}
              </span>
              <span>
                <strong className='block text-sm font-semibold'>
                  {t(step.title)}
                </strong>
                <span className='text-muted-foreground mt-1 block text-xs leading-5'>
                  {t(step.description)}
                </span>
              </span>
            </div>
          ))}
        </div>

        <div
          className='border-border mt-7 grid grid-cols-3 border-b'
          role='tablist'
          aria-label={t('Configuration scenarios')}
        >
          {SETUP_SCENES.map((scene) => {
            const Icon = scene.icon
            const selected = scene.id === sceneId
            return (
              <button
                key={scene.id}
                type='button'
                role='tab'
                aria-selected={selected}
                aria-controls='configuration-panel'
                tabIndex={selected ? 0 : -1}
                className={cn(
                  'relative flex min-h-12 items-center justify-center gap-2 px-3 text-xs font-semibold transition-colors duration-150 sm:text-sm',
                  selected
                    ? 'text-primary after:bg-primary after:absolute after:inset-x-0 after:-bottom-px after:h-0.5'
                    : 'text-muted-foreground hover:text-foreground'
                )}
                onClick={() => selectScene(scene.id)}
              >
                <Icon aria-hidden='true' className='size-4' />
                <span>{t(scene.label)}</span>
              </button>
            )
          })}
        </div>

        <div
          id='configuration-panel'
          role='tabpanel'
          className='home-business-card mt-5 grid overflow-hidden rounded-lg text-left md:grid-cols-[13.75rem_minmax(0,1fr)]'
        >
          <aside className='bg-muted border-border border-b p-4 md:border-r md:border-b-0'>
            <p className='text-muted-foreground mb-3 text-xs font-semibold'>
              {t('Tool examples')}
            </p>
            <div className='flex gap-2 overflow-x-auto md:grid'>
              {tools.map((tool) => {
                const active = tool.id === activeTool.id
                return (
                  <button
                    key={tool.id}
                    type='button'
                    className={cn(
                      'flex min-h-11 min-w-max items-center justify-between gap-3 rounded-md px-3 text-sm font-medium transition-colors duration-150 md:min-w-0',
                      active
                        ? 'bg-accent text-accent-foreground'
                        : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'
                    )}
                    aria-current={active ? 'true' : undefined}
                    onClick={() => setActiveToolId(tool.id)}
                  >
                    {tool.name}
                    <ChevronRight aria-hidden='true' className='size-4' />
                  </button>
                )
              })}
            </div>
          </aside>

          <div className='min-w-0 p-4 sm:p-6'>
            <div className='flex flex-col justify-between gap-4 sm:flex-row sm:items-start'>
              <div>
                <h3 className='text-base font-semibold'>
                  {t('{{tool}} configuration example', {
                    tool: activeTool.name,
                  })}
                </h3>
                <p className='text-muted-foreground mt-1 text-sm leading-6'>
                  {t(activeTool.summary)}
                </p>
              </div>
              <span className='bg-muted text-primary rounded-md px-3 py-2 text-xs font-semibold'>
                {t(
                  activeTool.protocol === 'openai'
                    ? 'OpenAI compatible'
                    : 'Anthropic compatible'
                )}
              </span>
            </div>

            <div className='mt-5 grid gap-3'>
              {[
                { label: 'Base URL', value: protocolBaseUrl },
                { label: 'API Key', value: 'sk-your-key' },
                { label: 'Model ID', value: '<available-model-id>' },
              ].map((field) => (
                <div
                  key={field.label}
                  className='grid gap-2 sm:grid-cols-[5.5rem_minmax(0,1fr)] sm:items-center'
                >
                  <span className='text-muted-foreground text-sm font-medium'>
                    {t(field.label)}
                  </span>
                  <span className='bg-muted border-border grid min-w-0 grid-cols-[minmax(0,1fr)_2.75rem] overflow-hidden rounded-md border'>
                    <code className='min-w-0 overflow-x-auto p-3 text-xs whitespace-nowrap'>
                      {field.value}
                    </code>
                    <button
                      type='button'
                      className='border-border text-muted-foreground hover:bg-accent hover:text-accent-foreground grid min-h-11 place-items-center border-l transition-colors duration-150'
                      aria-label={t('Copy {{label}}', {
                        label: t(field.label),
                      })}
                      onClick={() => copyValue(field.value)}
                    >
                      <Copy aria-hidden='true' className='size-4' />
                    </button>
                  </span>
                </div>
              ))}
            </div>

            <div className='text-muted-foreground mt-3 flex items-center gap-2 text-xs'>
              <Database aria-hidden='true' className='text-primary size-4' />
              {t('Service address loaded from the current deployment')}
            </div>

            <dl className='mt-5 grid gap-3 sm:grid-cols-3'>
              {[
                { label: 'Configuration location', value: activeTool.location },
                { label: 'Start or save', value: activeTool.action },
                {
                  label: 'Verification method',
                  value: activeTool.verification,
                },
              ].map((item) => (
                <div
                  key={item.label}
                  className='border-border bg-muted border-l-2 p-3'
                >
                  <dt className='text-muted-foreground text-xs font-medium'>
                    {t(item.label)}
                  </dt>
                  <dd className='mt-1 text-xs leading-5 font-semibold'>
                    {t(item.value)}
                  </dd>
                </div>
              ))}
            </dl>

            <div className='home-business-code mt-5 overflow-hidden rounded-md'>
              <div className='border-b border-white/10 px-4 py-2 text-xs font-semibold text-blue-100'>
                <span>{t('Full configuration')}</span>
                <button
                  type='button'
                  className='float-right flex min-h-8 items-center gap-2 rounded-md bg-white/10 px-3 text-white transition-colors duration-150 hover:bg-white/15'
                  onClick={() => copyValue(setupCode)}
                >
                  <Copy aria-hidden='true' className='size-3.5' />
                  {t('Copy')}
                </button>
              </div>
              <pre className='overflow-x-auto p-4 text-xs leading-6'>
                <code>{setupCode}</code>
              </pre>
            </div>

            <div className='border-border mt-5 flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
              <p className='home-business-success flex items-center gap-2 text-xs font-semibold'>
                <CheckCircle2 aria-hidden='true' className='size-4' />
                {t(
                  'After setup, send a minimal request and verify it in usage logs'
                )}
              </p>
              <a
                href='#troubleshooting'
                className='text-primary min-h-11 text-xs font-semibold sm:inline-flex sm:items-center'
              >
                {t('Configuration troubleshooting')}
              </a>
            </div>
            <p
              id='troubleshooting'
              className='border-border text-muted-foreground mt-3 border-t pt-3 text-xs leading-5'
            >
              <strong className='text-foreground font-semibold'>
                {t('Common issues:')}
              </strong>{' '}
              {t(
                'Incorrect Base URL path, invalid API Key, unavailable model, insufficient quota, rate limiting, or stale environment variables.'
              )}
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
