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
import { CodeXml, KeyRound, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const STEPS = [
  {
    title: 'Register or sign in',
    description: 'Create your platform account',
    icon: UserRound,
  },
  {
    title: 'Create API Key',
    description: 'Generate a dedicated, controllable credential',
    icon: KeyRound,
  },
  {
    title: 'Set Base URL and verify',
    description: 'Choose an available model and send a minimal request',
    icon: CodeXml,
  },
] as const

export function HowItWorks() {
  const { t } = useTranslation()

  return (
    <section
      id='quick-start'
      className='border-border bg-muted mx-auto grid w-[calc(100%-2rem)] max-w-6xl scroll-mt-20 overflow-hidden rounded-lg border sm:w-[calc(100%-3rem)] sm:grid-cols-3'
      aria-label={t('Quick start steps')}
    >
      {STEPS.map((step, index) => {
        const Icon = step.icon
        return (
          <article
            key={step.title}
            className='border-border grid grid-cols-[2rem_2rem_1fr] items-center gap-3 border-t p-5 first:border-t-0 sm:border-t-0 sm:border-l sm:first:border-l-0'
          >
            <span className='bg-primary text-primary-foreground grid size-8 place-items-center rounded-full text-sm font-semibold tabular-nums'>
              {index + 1}
            </span>
            <Icon aria-hidden='true' className='text-muted-foreground size-5' />
            <span>
              <strong className='block text-sm font-semibold'>
                {t(step.title)}
              </strong>
              <span className='text-muted-foreground mt-1 block text-xs leading-5'>
                {t(step.description)}
              </span>
            </span>
          </article>
        )
      })}
    </section>
  )
}
