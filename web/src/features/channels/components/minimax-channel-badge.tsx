import { useTranslation } from 'react-i18next'

import { ProviderBadge } from '@/components/provider-badge'

import { minimaxAccessLabel } from '../lib/minimax-management'
import type { Channel } from '../types'

export function MinimaxChannelBadge(props: {
  channel: Pick<Channel, 'type' | 'settings'>
}) {
  const { t } = useTranslation()
  return (
    <div className='min-w-0' title={`MiniMax (#${props.channel.type})`}>
      <ProviderBadge
        iconKey='Minimax.Color'
        label='MiniMax'
        copyable={false}
        colorText={false}
        showDot={false}
      />
      <div className='text-muted-foreground text-xs'>
        {t(minimaxAccessLabel(props.channel))}
      </div>
    </div>
  )
}
