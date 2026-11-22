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

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

type PluginKindSwitchProps<Kind extends string> = {
  value: Kind
  options: readonly { value: Kind; label: string }[]
  counts: Record<Kind, number>
  onValueChange: (kind: Kind) => void
}

/** A segmented switch: the chosen item is raised on a muted track. */
export const SEGMENTED_SWITCH_CLASS =
  'bg-muted h-8 max-w-full shrink-0 overflow-x-auto rounded-lg p-[3px]'
export const SEGMENTED_SWITCH_ITEM_CLASS =
  'text-muted-foreground hover:text-foreground aria-pressed:bg-background aria-pressed:text-foreground dark:aria-pressed:bg-input/40 h-full gap-1.5 rounded-md px-2.5 hover:bg-transparent aria-pressed:shadow-sm'

export function PluginKindSwitch<Kind extends string>(
  props: PluginKindSwitchProps<Kind>
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <ToggleGroup
      value={[props.value]}
      onValueChange={(value) => {
        const next = props.options.find(
          (option) =>
            option.value !== props.value && value.includes(option.value)
        )
        if (next) props.onValueChange(next.value)
      }}
      aria-label={t('Plugin kind')}
      spacing={1}
      className={SEGMENTED_SWITCH_CLASS}
    >
      {props.options.map(({ value: kind, label }) => (
        <ToggleGroupItem
          key={kind}
          value={kind}
          className={SEGMENTED_SWITCH_ITEM_CLASS}
        >
          {t(label)}{' '}
          <span className='bg-foreground/[0.06] group-aria-pressed/toggle:bg-primary/10 group-aria-pressed/toggle:text-primary min-w-5 rounded-full px-1.5 text-center text-xs tabular-nums'>
            {formatNumber(props.counts[kind], locale)}
          </span>
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}
