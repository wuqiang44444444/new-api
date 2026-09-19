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

import { DetailRow } from '@/features/usage-logs/components/dialogs/log-detail-layout'

import { AUTO_CHECK_DETAIL_KEYS } from './channel-check-detail-keys'

function autoCheckDetailLabel(key: string, t: (key: string) => string): string {
  switch (key) {
    case 'connection_result':
      return t('Connection observation')
    case 'probe_media':
      return t('Probe target')
    case 'check_result':
      return t('Check result')
    case 'check_reason':
      return t('Check failure reason')
    case 'check_code':
      return t('Check error code')
    case 'upstream_status':
      return t('Upstream HTTP status')
    case 'check_scope':
      return t('Check scope')
    case 'config_check':
      return t('Config check')
    case 'generation_evidence':
      return t('Generation evidence')
    case 'upstream_request':
      return t('Upstream request')
    case 'config_reason':
      return t('Failure reason')
    case 'config_entry':
      return t('Admin entry')
    case 'config_summary':
      return t('Diagnostic summary')
    case 'readonly_check':
      return t('Read-only check')
    case 'billing_model':
      return t('Billing model')
    default:
      return key
  }
}

function autoCheckDetailValue(
  key: string,
  value: string,
  t: (key: string) => string
): string {
  switch (key) {
    case 'probe_media':
      if (value === 'image') return t('Image probe')
      if (value === 'video') return t('Video probe')
      return value
    case 'connection_result':
      if (value === 'auth_error') {
        return t('Response received; authentication or permission issue')
      }
      if (value === 'rate_limited') return t('Response received; rate limited')
      if (value === 'service_error') return t('Service or gateway issue')
      if (value === 'probe_unsupported') {
        return t('Response received; probe not applicable')
      }
      if (value === 'response_received') return t('Response received')
      if (value === 'connection_error') return t('Connection error')
      return t('Connection not verified')
    case 'check_result':
      if (value === 'passed') return t('Passed')
      if (value === 'failed') return t('Failed')
      if (value === 'unsupported') return t('Unsupported')
      return value
    case 'check_reason':
      switch (value) {
        case 'test_config_error':
          return t('Configuration check failed')
        case 'test_request_build_failed':
          return t('Request preparation failed')
        case 'test_upstream_unreachable':
          return t('Upstream connection failed')
        case 'test_upstream_rejected':
          return t('Upstream rejected the probe')
        case 'response_time_exceeded':
          return t('Response time threshold exceeded')
        case 'asset_probe_failed':
          return t('Asset read-only probe failed')
        case 'test_azure_batch_connection_failed':
          return t('Read-only connection check failed')
        case 'test_unsupported_channel_type':
          return t('Channel test unsupported')
        case 'unclassified':
          return t('Check failed without a classified reason')
        default:
          return value
      }
    case 'check_scope':
      if (value === 'config_only') return t('Local config check only')
      if (value === 'readonly_probe') return t('Read-only probe')
      if (value === 'target_unresolved') {
        return t('Check target needs confirmation')
      }
      if (value === 'generation_probe') return t('Generation probe')
      return value
    case 'config_check':
      if (value === 'failed') return t('Failed')
      if (value === 'passed') return t('Passed')
      if (value === 'not_checked') return t('Not checked')
      return value
    case 'generation_evidence':
      return value === 'not_verified' ? t('Not verified this run') : value
    case 'upstream_request':
      if (value === 'not_sent') return t('Not sent')
      if (value === 'attempted') return t('Attempted; no response received')
      if (value === 'response_received') return t('Response received')
      return value
    case 'readonly_check':
      if (value === 'unsupported') return t('Read-only check unsupported')
      if (value === 'passed') return t('Passed')
      if (value === 'failed') return t('Failed')
      return t('Not checked')
    case 'config_reason':
      switch (value) {
        case 'price_not_configured':
          return t('Model price not configured')
        case 'billing_expr_missing':
          return t('Billing expression missing')
        case 'billing_expr_failed':
          return t('Billing expression evaluation failed')
        case 'billing_input_invalid':
          return t('Billing input invalid')
        case 'image_price_not_configured':
          return t('Image price not configured')
        case 'billing_usage_unavailable':
          return t('Required billing usage unavailable')
        case 'billing_expr_required':
          return t('Billing expression required')
        case 'billing_validation_failed':
          return t('Billing validation failed')
        case 'target_ambiguous':
          return t('Check target needs confirmation')
        case 'protocol_invalid':
          return t('Channel protocol configuration invalid')
        case 'parameter_override_invalid':
          return t('Channel parameter overrides invalid')
        case 'model_mapping_invalid':
          return t('Model mapping invalid')
        default:
          return value
      }
    case 'config_entry':
      switch (value) {
        case 'model_pricing':
          return t('Model pricing settings')
        case 'billing_expression':
          return t('Billing expression settings')
        case 'channel_edit':
          return t('Channel edit')
        default:
          return value
      }
    default:
      return value
  }
}

function diagnosticSummary(
  reason: string | undefined,
  t: (key: string) => string
): string {
  switch (reason) {
    case 'price_not_configured':
      return t('Configure a price for the billing model.')
    case 'image_price_not_configured':
      return t('Configure a per-image price or a billing expression.')
    case 'billing_expr_missing':
      return t('Configure the required billing expression.')
    case 'billing_expr_required':
      return t('Configure the required billing expression.')
    case 'billing_expr_failed':
      return t('Check the billing expression and its required inputs.')
    case 'billing_usage_unavailable':
      return t(
        'The adapter cannot provide the usage required by this expression.'
      )
    case 'billing_input_invalid':
      return t('Check billing inputs and quota limits.')
    case 'model_mapping_invalid':
      return t('Correct the channel model mapping.')
    case 'parameter_override_invalid':
      return t('Correct the channel parameter overrides.')
    case 'protocol_invalid':
      return t('Check the channel protocol configuration.')
    case 'target_ambiguous':
      return t('Confirm the automatic check target in channel settings.')
    case 'billing_validation_failed':
      return t('Billing validation failed. Check the billing model settings.')
    default:
      return t(
        'Configuration diagnostic unavailable. Review the failure reason and channel settings.'
      )
  }
}

export function ChannelCheckDetails(props: { detail: Record<string, string> }) {
  const { t } = useTranslation()
  return (
    <>
      {AUTO_CHECK_DETAIL_KEYS.filter((key) => props.detail[key]).map((key) => {
        const value =
          key === 'config_summary'
            ? diagnosticSummary(props.detail.config_reason, t)
            : autoCheckDetailValue(key, props.detail[key], t)
        // Only local, known management destinations can become links.
        let destination: string | undefined
        if (key === 'config_entry') {
          if (props.detail[key] === 'channel_edit') {
            destination = '/channels'
          } else if (
            ['model_pricing', 'billing_expression'].includes(props.detail[key])
          ) {
            destination = '/system-settings/billing/model-pricing'
          }
        }
        return (
          <DetailRow
            key={key}
            label={autoCheckDetailLabel(key, t)}
            value={
              destination ? (
                <a
                  href={destination}
                  className='text-primary underline underline-offset-4 focus-visible:outline-2'
                >
                  {value}
                </a>
              ) : (
                value
              )
            }
          />
        )
      })}
    </>
  )
}
