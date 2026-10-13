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
import type { LogOtherData } from '../types'

export interface TaskCostSummary {
  beforeLabel: 'Pre-consumed cost' | 'Cost before adjustment'
  before: number | null
  changeLabel: 'Refund this event' | 'Additional charge this event'
  change: number
  afterLabel: 'Final cost' | 'Cost after adjustment'
  after: number | null
  refund: boolean
}

// This is an event snapshot, never a recomputation of the task's current bill.
export function buildTaskCostSummary(
  logType: number | undefined,
  quota: number,
  other: LogOtherData | null
): TaskCostSummary | null {
  if (!other?.task_id || (logType !== 2 && logType !== 6)) return null
  if (!Number.isSafeInteger(quota) || quota < 0) return null
  const refund = logType === 6
  const event = other.task_billing_event
  if (!refund && event !== 'adjustment') return null

  const adjustment = event === 'adjustment'
  const beforeValue = adjustment
    ? other.pre_consumed_quota
    : other.adjustment_before_quota
  const afterValue =
    event === 'customer_refund' ? other.net_quota : other.actual_quota
  let before = null
  let after = null
  if (
    typeof beforeValue === 'number' &&
    Number.isSafeInteger(beforeValue) &&
    beforeValue >= 0
  ) {
    before = beforeValue
  }
  if (
    typeof afterValue === 'number' &&
    Number.isSafeInteger(afterValue) &&
    afterValue >= 0
  ) {
    after = afterValue
  }
  if (
    before !== null &&
    after !== null &&
    (refund ? before - after : after - before) !== quota
  ) {
    before = null
    after = null
  }

  return {
    beforeLabel: adjustment ? 'Pre-consumed cost' : 'Cost before adjustment',
    before,
    changeLabel: refund ? 'Refund this event' : 'Additional charge this event',
    change: quota,
    afterLabel:
      event === 'customer_refund' ? 'Cost after adjustment' : 'Final cost',
    after,
    refund,
  }
}

export function taskRefundLabelKey(other: LogOtherData | null): string {
  switch (other?.task_billing_event) {
    case 'adjustment':
      return 'Task settlement refund'
    case 'refund':
      return 'Task failure refund'
    case 'customer_refund':
      return 'Manual task refund'
    default:
      return 'Async task refund'
  }
}
