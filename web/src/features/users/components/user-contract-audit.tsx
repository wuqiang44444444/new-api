import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { formatTimestamp } from '@/lib/format'

import { getContractEntityAudits } from '../api'
import type { CustomerContractAudit } from '../types'

function AuditOperation(props: {
  operation: CustomerContractAudit['operation']
}) {
  const { t } = useTranslation()
  switch (props.operation) {
    case 'create':
      return <span>{t('Created')}</span>
    case 'enable':
      return <span>{t('Enabled')}</span>
    case 'disable':
      return <span>{t('Disabled')}</span>
    case 'migrate':
      return <span>{t('Migrated')}</span>
    default:
      return <span>{t('Updated')}</span>
  }
}

type CustomerContractAuditProps = {
  contractId: number
  audits: CustomerContractAudit[]
  page: number
  total: number
  setAudits: Dispatch<SetStateAction<CustomerContractAudit[]>>
  setPage: Dispatch<SetStateAction<number>>
}

export function CustomerContractAuditHistory(props: CustomerContractAuditProps) {
  const { t } = useTranslation()
  if (props.audits.length === 0) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyTitle>{t('No contract changes yet')}</EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  }

  const pageCount = Math.ceil(props.total / 20)
  const loadPage = (nextPage: number) => {
    void getContractEntityAudits(props.contractId, nextPage).then(
      (response) => {
        if (response.success && response.data) {
          props.setAudits(response.data.items)
          props.setPage(nextPage)
        }
      }
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      {props.audits.map((audit) => (
        <div key={audit.id} className='rounded-lg border p-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <div className='font-medium'>
              v{audit.contract_version} ·{' '}
              <AuditOperation operation={audit.operation} />
            </div>
            <div className='text-muted-foreground text-xs'>
              {formatTimestamp(audit.created_at)}
            </div>
          </div>
          <div className='text-muted-foreground mt-1 text-sm'>
            {audit.admin_username || `#${audit.admin_user_id}`} ·{' '}
            {audit.before_rule_count} → {audit.after_rule_count} {t('rules')}
          </div>
          <div className='mt-2 text-sm'>{audit.reason}</div>
        </div>
      ))}
      {pageCount > 1 && (
        <div className='flex items-center justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page <= 1}
            onClick={() => loadPage(props.page - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {props.page} / {pageCount}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={props.page >= pageCount}
            onClick={() => loadPage(props.page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      )}
    </div>
  )
}
