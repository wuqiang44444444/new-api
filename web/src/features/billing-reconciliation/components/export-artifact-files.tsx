import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import {
  downloadExportFile,
  requestExportDownload,
  type CustomerExportArtifactFileDto,
} from '../export-api'

export function ExportArtifactFiles(props: {
  jobId: string
  files: CustomerExportArtifactFileDto[]
}) {
  const { t } = useTranslation()
  const download = useMutation({
    mutationFn: async (filename: string) => {
      const result = await requestExportDownload(props.jobId)
      const file = result.files.find((item) => item.file_name === filename)
      if (!file) throw new Error('Export file unavailable')
      downloadExportFile(file.url)
    },
  })
  const singleFile = props.files.length === 1
  const fileButton = (file: CustomerExportArtifactFileDto) => {
    let label = singleFile
      ? t('Download CSV')
      : t('Download {{file}}', { file: file.file_name })
    if (download.isPending && download.variables === file.file_name) {
      label = t('Loading...')
    }
    return (
      <Button
        size='sm'
        variant={singleFile ? 'default' : 'outline'}
        className='h-auto min-h-9 max-w-full break-all whitespace-normal'
        disabled={download.isPending}
        onClick={() => download.mutate(file.file_name)}
      >
        {label}
      </Button>
    )
  }

  return (
    <div className='mt-2 space-y-2'>
      {singleFile ? (
        fileButton(props.files[0])
      ) : (
        <Collapsible>
          <CollapsibleTrigger
            render={
              <Button size='sm' variant='outline' className='group min-h-9' />
            }
          >
            {t('View files ({{count}})', { count: props.files.length })}
            <HugeiconsIcon
              icon={ArrowDown01Icon}
              aria-hidden='true'
              className='transition-transform group-aria-expanded:rotate-180'
            />
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className='mt-2 space-y-2'>
              {props.files.map((file) => (
                <li key={file.file_name}>{fileButton(file)}</li>
              ))}
            </ul>
          </CollapsibleContent>
        </Collapsible>
      )}
      {download.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to download export.')}
        </p>
      )}
    </div>
  )
}
