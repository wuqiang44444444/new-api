import { expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { getAdminUpstreamDetails } from '../api'
import {
  createSelfExport,
  resubmitExport,
  type CustomerExportJobView,
} from '../export-api'
import {
  createAdminVersion,
  createAdminVersionCorrection,
  verifyAdminStatementSource,
} from '../version-api'

const get = vi.hoisted(() =>
  vi.fn().mockResolvedValue({ data: { success: true, data: {} } })
)
const post = vi.hoisted(() =>
  vi.fn().mockResolvedValue({ data: { success: true, data: {} } })
)
vi.mock('@/lib/api', () => ({ api: { post, get } }))

it('registers source verification through the existing scoped admin endpoint', async () => {
  await verifyAdminStatementSource(
    { user_id: 91, start_timestamp: 1785513600, end_timestamp: 1788191999 },
    {
      fingerprint: 'snapshot',
      backup_evidence: 'verified backup AUDIT-8',
      retention_evidence: 'retention checked',
    }
  )
  expect(post).toHaveBeenLastCalledWith(
    '/api/billing/admin/customer-statement-source-verification',
    {
      fingerprint: 'snapshot',
      backup_evidence: 'verified backup AUDIT-8',
      retention_evidence: 'retention checked',
    },
    {
      params: {
        user_id: 91,
        start_timestamp: 1785513600,
        end_timestamp: 1788191999,
      },
    }
  )
})

it('regenerates the same customer and full scope including explicit key zero', async () => {
  const filters: CustomerExportJobView['filters'] = {
    field_version: 2,
    start_timestamp: 1000,
    end_timestamp: 2001,
    timezone: 'Asia/Shanghai',
    token_id: 0,
    channel_id: 3,
    token_name: 'key',
    group: 'vip',
    request_id: 'req',
    upstream_request_id: 'up',
    username: 'customer',
    model_name: 'm',
    billing_mode: 'token',
    log_types: [2],
  }
  const job: CustomerExportJobView = {
    job_id: 'job',
    user_id: 1,
    target_user_id: 27,
    job_type: 'statement_details',
    status: 'failed',
    filters,
    progress: { scanned: 0, matched: 0, written: 0, files: 0 },
    cancel_requested: false,
    created_at: 1000,
  }
  await resubmitExport(job)
  expect(post).toHaveBeenCalledWith(
    '/api/billing/admin/customer-exports',
    expect.objectContaining({
      job_type: 'statement_details',
      start_timestamp: 1000,
      end_timestamp: 2001,
      token_id: 0,
      channel_id: 3,
      token_name: 'key',
      group: 'vip',
      request_id: 'req',
      upstream_request_id: 'up',
      username: 'customer',
      model_name: 'm',
      billing_mode: 'token',
      log_types: [2],
    }),
    { params: { user_id: 27 } }
  )
})

it.each([
  ['zhCN', 'zh'],
  ['zhTW', 'zh-TW'],
  ['en', 'en'],
])(
  'uses %s consistently in exports and frozen versions',
  async (language, expected) => {
    const previous = i18n.language
    await i18n.changeLanguage(language)
    try {
      const period = { start_timestamp: 1788192000, end_timestamp: 1790783999 }
      await createSelfExport({ ...period, job_type: 'statement_summary' })
      expect(post).toHaveBeenLastCalledWith(
        '/api/billing/exports',
        expect.objectContaining({ language: expected })
      )
      await createAdminVersion({ ...period, user_id: 7 })
      expect(post).toHaveBeenLastCalledWith(
        '/api/billing/admin/customer-statement-versions',
        null,
        { params: { ...period, user_id: 7, language: expected } }
      )
      await createAdminVersionCorrection(
        { ...period, user_id: 7 },
        { public_reason: 'Correction' }
      )
      expect(post).toHaveBeenLastCalledWith(
        '/api/billing/admin/customer-statement-versions/correction',
        { public_reason: 'Correction' },
        { params: { ...period, user_id: 7, language: expected } }
      )
    } finally {
      await i18n.changeLanguage(previous)
    }
  }
)

it('sends the backend page parameter for upstream details', async () => {
  await getAdminUpstreamDetails({
    start_timestamp: 1000,
    end_timestamp: 2000,
    channel_id: 3,
    page: 2,
    page_size: 50,
  })
  expect(get).toHaveBeenLastCalledWith('/api/billing/admin/upstream-details', {
    params: expect.objectContaining({ p: 2, page: undefined, page_size: 50 }),
  })
})
it('regenerates upstream exports from their authorized frozen job scope', async () => {
  await resubmitExport({
    job_id: 'cex_fixture',
    job_type: 'upstream_details',
    filters: {
      field_version: 11,
      start_timestamp: 1000,
      end_timestamp: 2000,
      timezone: 'Asia/Shanghai',
    },
  } as CustomerExportJobView)
  expect(post).toHaveBeenLastCalledWith('/api/billing/admin/upstream-exports', {
    source_job_id: 'cex_fixture',
  })
})
