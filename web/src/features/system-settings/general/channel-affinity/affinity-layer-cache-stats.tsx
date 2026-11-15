import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'

import { clearAffinityLayer } from './api'
import type { CacheStats } from './types'

type Layer = 'session' | 'api_key' | 'user'

export function AffinityLayerCacheStats({
  stats,
  onCleared,
}: {
  stats: CacheStats
  onCleared: () => void
}) {
  const { t, i18n } = useTranslation()
  const [selectedLayer, setSelectedLayer] = useState<Layer | null>(null)
  const mutation = useMutation({
    mutationFn: async (layer: Layer) => {
      const result = await clearAffinityLayer(layer)
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
    },
    onSuccess: () => {
      toast.success(t('Cleared'))
      setSelectedLayer(null)
      onCleared()
    },
    onError: (error) => handleServerError(error),
  })
  const layers = [
    {
      id: 'session',
      label: t('Session'),
      total: stats.stats_available === false ? null : stats.total,
    },
    { id: 'api_key', label: t('API Key'), total: stats.api_key_entries },
    { id: 'user', label: t('User'), total: stats.user_entries },
  ] as const
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <section
      aria-label={t('Affinity cache layers')}
      className='space-y-2 rounded-xl border p-4'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-semibold'>{t('Affinity cache layers')}</h3>
        <span className='text-muted-foreground text-xs'>
          {stats.shared ? t('Shared cache') : t('Local cache')}
          {stats.refreshed_at
            ? ` · ${new Date(stats.refreshed_at).toLocaleString(locale)}`
            : ''}
        </span>
      </div>
      {layers.map(({ id, label, total }) => (
        <div
          key={id}
          className='flex flex-wrap items-center justify-between gap-2 border-t pt-2 text-sm'
        >
          <span>{label}</span>
          <div className='flex items-center gap-3'>
            <span className='tabular-nums'>
              {total == null ? t('Unavailable') : total.toLocaleString(locale)}
            </span>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => setSelectedLayer(id)}
            >
              {t('Clear cache')}
            </Button>
          </div>
        </div>
      ))}
      {selectedLayer && (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setSelectedLayer(null)
          }}
          title={t('Clear cache')}
          desc={t('Clear the selected affinity layer?')}
          confirmText={t('Clear cache')}
          destructive
          isLoading={mutation.isPending}
          handleConfirm={() => mutation.mutate(selectedLayer)}
        />
      )}
    </section>
  )
}
