import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { formatTimestamp } from '@/lib/format'

import { getCustomerContractAudits } from '../api'
import type { CustomerContractAudit } from '../types'

function AuditOperation({
  operation,
}: {
  operation: CustomerContractAudit['operation']
}) {
  const { t } = useTranslation()
  switch (operation) {
    case 'create':
      return <>{t('Created')}</>
    case 'enable':
      return <>{t('Enabled')}</>
    case 'disable':
      return <>{t('Disabled')}</>
    default:
      return <>{t('Updated')}</>
  }
}

type CustomerContractAuditProps = {
  userId: number
  audits: CustomerContractAudit[]
  page: number
  total: number
  setAudits: Dispatch<SetStateAction<CustomerContractAudit[]>>
  setPage: Dispatch<SetStateAction<number>>
}

export function CustomerContractAuditHistory({
  userId,
  audits,
  page,
  total,
  setAudits,
  setPage,
}: CustomerContractAuditProps) {
  const { t } = useTranslation()
  if (audits.length === 0) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyTitle>{t('No contract changes yet')}</EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  }

  const pageCount = Math.ceil(total / 20)
  const loadPage = (nextPage: number) => {
    void getCustomerContractAudits(userId, nextPage).then((response) => {
      if (response.success && response.data) {
        setAudits(response.data.items)
        setPage(nextPage)
      }
    })
  }

  return (
    <div className='flex flex-col gap-3'>
      {audits.map((audit) => (
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
            disabled={page <= 1}
            onClick={() => loadPage(page - 1)}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {page} / {pageCount}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page >= pageCount}
            onClick={() => loadPage(page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      )}
    </div>
  )
}
