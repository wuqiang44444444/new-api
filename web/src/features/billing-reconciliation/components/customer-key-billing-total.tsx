import { useTranslation } from 'react-i18next'

import {
  TableCell,
  TableFooter,
  TableHead,
  TableRow,
} from '@/components/ui/table'

export function CustomerKeyBillingTotal(props: {
  requests: string
  original: string
  savings: string
  net: string
}) {
  const { t } = useTranslation()

  return (
    <TableFooter>
      <TableRow>
        <TableHead scope='row'>{t('Total')}</TableHead>
        <TableCell className='text-right'>{props.requests}</TableCell>
        <TableCell className='text-right'>{props.original}</TableCell>
        <TableCell className='text-right'>{props.savings}</TableCell>
        <TableCell className='text-right font-semibold!'>{props.net}</TableCell>
      </TableRow>
    </TableFooter>
  )
}
