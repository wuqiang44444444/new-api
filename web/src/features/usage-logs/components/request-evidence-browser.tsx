import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { TaskEvidence } from './task-evidence'

export function RequestEvidenceBrowser({ isRoot }: { isRoot: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [input, setInput] = useState('')
  const [requestId, setRequestId] = useState('')
  return (
    <>
      <Button variant='outline' onClick={() => setOpen(true)}>
        {t('Request evidence')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Request evidence')}
        contentClassName='sm:max-w-3xl'
      >
        {open && (
          <div className='max-h-[70vh] space-y-3 overflow-auto'>
            <form
              className='flex gap-2'
              onSubmit={(event) => {
                event.preventDefault()
                setRequestId(input.trim())
              }}
            >
              <Input
                aria-label={t('Request ID')}
                placeholder={t('Request ID')}
                value={input}
                onChange={(event) => setInput(event.target.value)}
              />
              <Button type='submit'>{t('Search')}</Button>
            </form>
            <TaskEvidence
              key={requestId}
              browse
              requestId={requestId || undefined}
              isRoot={isRoot}
            />
          </div>
        )}
      </Dialog>
    </>
  )
}
