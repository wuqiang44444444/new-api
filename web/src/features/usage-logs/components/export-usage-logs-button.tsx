import { Download01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
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
import { getRouteApi } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  createSelfExport,
  createAdminExport,
} from '@/features/billing-reconciliation/export-api'
import { ExportJobsDrawer } from '@/features/billing-reconciliation/export-jobs-drawer'

import { buildApiParams } from '../lib/utils'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

/**
 * 使用记录导出入口：按当前筛选创建异步导出任务。导出遵循当前筛选并包含
 * 全部匹配记录（不限当前页）；大文件后台分批生成，在导出记录抽屉下载。
 */
export function ExportUsageLogsButton() {
  const { t } = useTranslation()
  const searchParams = route.useSearch()
  const { isAdminView } = useLogsViewScope()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const submitExport = async () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams,
      isAdmin: isAdminView,
    })
    const start = params.start_timestamp
    // Existing log APIs include the final second; exports use [start, end).
    const end =
      params.end_timestamp == null ? undefined : params.end_timestamp + 1
    if (
      start == null ||
      end == null ||
      end <= start ||
      end - start > 31 * 24 * 3600
    ) {
      toast.error(
        t('Usage record exports cover at most 31 days. Narrow the time range.')
      )
      return
    }
    const target = params.billing_statement ? params.user_id : params.username
    if (isAdminView && !target) {
      toast.error(t('Select customer'))
      return
    }
    setSubmitting(true)
    try {
      const payload = {
        job_type: params.billing_statement
          ? ('statement_details' as const)
          : ('usage_logs' as const),
        start_timestamp: start,
        end_timestamp: end,
        log_types: params.type ? [params.type] : undefined,
        model_name: params.model_name,
        token_id: params.token_id,
        token_name: params.token_name,
        billing_mode: params.billing_mode,
        channel_id:
          params.billing_statement || params.channel
            ? params.channel
            : undefined,
        group: params.group,
        request_id: params.request_id,
        upstream_request_id: params.upstream_request_id,
        username: params.username,
      }
      if (isAdminView && target) {
        await createAdminExport(target, payload)
      } else {
        await createSelfExport(payload)
      }
      toast.success(t('Export submitted. Track it in export jobs.'))
      setDrawerOpen(true)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to submit export.')
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Button
        variant='outline'
        size='sm'
        disabled={submitting}
        onClick={() => void submitExport()}
      >
        <HugeiconsIcon
          icon={Download01Icon}
          strokeWidth={2}
          data-icon='inline-start'
        />
        {t('Export usage records')}
      </Button>
      <ExportJobsDrawer open={drawerOpen} onOpenChange={setDrawerOpen} />
    </>
  )
}
