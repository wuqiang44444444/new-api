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
import { Boxes, FileSearch, Route, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const VALUES = [
  {
    title: 'Unified multi-model access',
    description:
      'Use one API and credential system for the text, image, audio, and video models currently available on the platform.',
    icon: Boxes,
  },
  {
    title: 'Routing reliability',
    description:
      'Multi-channel scheduling, failure retries, and availability controls reduce dependence on any single upstream channel.',
    icon: Route,
  },
  {
    title: 'Cost and usage governance',
    description:
      'Manage quotas, rate limits, and transparent billing centrally so every model request remains measurable.',
    icon: SlidersHorizontal,
  },
  {
    title: 'Logs and observability',
    description:
      'Review models, token usage, and billing outcomes to support troubleshooting and ongoing operations.',
    icon: FileSearch,
  },
] as const

export function Features() {
  const { t } = useTranslation()

  return (
    <section
      id='models'
      className='mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-24'
      aria-labelledby='home-values-title'
    >
      <div className='max-w-2xl'>
        <p className='text-primary text-sm font-semibold'>
          {t('From integration to continuous operations')}
        </p>
        <h2
          id='home-values-title'
          className='mt-3 text-[clamp(1.75rem,3vw,2.375rem)] leading-tight font-semibold tracking-[-0.035em]'
        >
          {t('Reliable, approachable, and truly governable')}
        </h2>
        <p className='text-muted-foreground mt-4 text-base leading-7'>
          {t(
            'More than a model proxy, the gateway brings reliability, cost control, and request governance into one operational entry point.'
          )}
        </p>
      </div>

      <div className='mt-9 grid gap-7 sm:grid-cols-2 lg:grid-cols-4'>
        {VALUES.map((value) => {
          const Icon = value.icon
          return (
            <article
              key={value.title}
              className='border-border hover:border-primary border-t-2 pt-6 transition-colors duration-150'
            >
              <span className='bg-accent text-accent-foreground grid size-9 place-items-center rounded-md'>
                <Icon aria-hidden='true' className='size-[1.125rem]' />
              </span>
              <h3 className='mt-5 text-base font-semibold'>{t(value.title)}</h3>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {t(value.description)}
              </p>
            </article>
          )
        })}
      </div>
    </section>
  )
}
