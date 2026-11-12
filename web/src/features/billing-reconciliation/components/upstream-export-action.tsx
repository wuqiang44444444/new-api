import { useTranslation } from 'react-i18next'

import {
  createUpstreamExport,
  type CustomerExportSubmitPayload,
} from '../export-api'
import { BillingExportAction } from './billing-export-action'

export function UpstreamExportAction(props: {
  payload: CustomerExportSubmitPayload
  scope: string[]
  available: boolean
  unavailableNotice?: string
  className?: string
}) {
  const { t } = useTranslation()
  return (
    <BillingExportAction
      upstreamBilling
      startTimestamp={props.payload.start_timestamp}
      statement={t('Upstream details')}
      scope={props.scope}
      description={t(
        'The CSV includes all records matching these filters across all pages.'
      )}
      notice={t(
        'A matching export is reused when its file is still available.'
      )}
      available={props.available}
      unavailableNotice={props.unavailableNotice}
      className={props.className}
      submit={() => createUpstreamExport(props.payload)}
    />
  )
}
