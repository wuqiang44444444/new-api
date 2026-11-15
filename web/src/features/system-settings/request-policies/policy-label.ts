import type { TFunction } from 'i18next'

// Labels for the decision events recorded by the backend request policy state
// (reasons, sources and health), the session behaviors, and the auto-disable
// sources reported by the channel operations summary.
export function policyLabel(t: TFunction, value: string): string {
  switch (value) {
    case 'channel_failover':
      return t('Channel failover')
    case 'bound_channel_temporarily_unavailable':
      return t('Bound channel is temporarily unavailable')
    case 'bound_channel_would_be_bypassed':
      return t('Bound channel would be bypassed (observe only)')
    case 'recovery_trial_success':
      return t('Recovery trial succeeded')
    case 'recovery_trial_failed':
      return t('Recovery trial failed')
    case 'recovery_trial_unconfirmed':
      return t('Recovery trial unconfirmed')
    case 'recovery_trial_unrecorded':
      return t('Recovery trial result not recorded')
    case 'contract_route_unavailable':
      return t('Contract routes unavailable')
    case 'eligible_failover_channel':
    case 'eligible_backup':
      return t('Eligible backup channel')
    case 'binding_updated_after_success':
      return t('Binding updated after successful failover')
    case 'no_eligible_backup':
      return t('No eligible backup channel')
    case 'original_channel_required':
      return t('Request requires its original channel')
    case 'response_started_or_cancelled':
      return t('Response already started or request cancelled')
    case 'migration_rejected':
      return t('Backup rejected the migrated request')
    case 'failover_attempt_used':
      return t('Failover attempt already used')
    case 'probe_recovery_trial':
      return t('Controlled recovery trial')
    case 'upstream_unavailable_sample':
      return t('Upstream failure recorded')
    case 'health_unknown':
      return t('Channel health unavailable')
    case 'migration_not_verified':
      return t('Request content is outside the verified migration scope')
    case 'upstream_failure':
      return t('Upstream request failed')
    case 'request_failed':
      return t('Request failed')
    case 'task_failed':
      return t('Task failed')
    case 'strict_session':
      return t('Session behavior prevents switching channels')
    case 'attempt_budget_exhausted':
      return t('Total attempt budget exhausted')
    case 'channel_error':
      return t('Channel error')
    case 'unrecognized_status':
      return t('Unrecognized status code')
    case 'task_accepted':
      return t('The task was accepted; submission cannot be replayed')
    case 'pinned_channel':
      return t('Fixed channel constraint')
    case 'local_rejection':
      return t('Request rejected locally')
    case 'non_retryable_error':
    case 'system_retry_exclusion':
      return t('System retry exclusion')
    case 'status_not_retryable':
      return t('Status is outside the retry rules')
    case 'retry_status_matched':
      return t('Status matches the retry rules')
    case 'session_rule_matched':
      return t('Session rule matched')
    case 'channel_selected':
      return t('Channel selected')
    case 'request_completed':
      return t('Request completed')
    case 'stream_not_successful':
      return t('Stream did not complete successfully')
    case 'channel':
      return t('Channel')
    case 'global':
      return t('Global default')
    case 'global_and_channel':
      return t('Global and channel settings')
    case 'session_rule':
      return t('Session rule')
    case 'system':
      return t('System constraint')
    case 'routing':
      return t('Channel selection')
    case 'upstream':
      return t('Upstream response')
    case 'transport':
      return t('Upstream connection')
    case 'local':
      return t('Local check')
    case 'channel_constraint':
      return t('Fixed channel constraint')
    case 'unchanged':
      return t('Unchanged')
    case 'channel_disable_requested':
      return t('Channel disable requested')
    case 'key_disable_requested':
      return t('Current key disable requested')
    case 'off':
    case '':
      return t('Do not keep sessions')
    case 'prefer':
      return t('Prefer the original channel, allow switching')
    case 'strict':
      return t('Require the original channel')
    default:
      return value
  }
}
