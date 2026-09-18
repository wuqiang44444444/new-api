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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { BillingDataQuality } from '../types'
import {
  abandonAdminVersion,
  cleanupAdminVersion,
  confirmAdminVersion,
  createAdminVersion,
  createAdminVersionCorrection,
  getAdminVersionDiff,
  getAdminVersionDownload,
  getAdminVersionMonthStatus,
  getSelfVersionDownload,
  getSelfVersionMonthStatus,
  type BillingStatementVersionDiff,
  type BillingStatementVersionInfo,
} from '../version-api'
import { StatementSourceVerification } from './statement-source-verification'
import { StatementVersionDiffDetails } from './statement-version-diff-details'

type StatementVersionPanelProps = {
  isAdmin: boolean
  userId?: number
  period: { start_timestamp: number; end_timestamp: number }
  previewVersionId: string | null
  onPreviewChange: (draftPublicId: string | null) => void
  onChanged: () => void
}

// 版本状态条与操作面板（docs/80-dev/2026-09-17 方案 6.1/6.2）。
// 管理员：生成/预览/放弃/确认/更正/历史/下载/清理；客户：当前版本/历史/下载。
export function StatementVersionPanel(props: StatementVersionPanelProps) {
  const { t } = useTranslation()
  const { isAdmin, userId, period } = props
  const queryClient = useQueryClient()
  const [historyOpen, setHistoryOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [correctionOpen, setCorrectionOpen] = useState(false)

  const statusQuery = useQuery({
    queryKey: [
      'billing-statement-version-month-status',
      isAdmin,
      userId,
      period.start_timestamp,
    ],
    queryFn: async () => {
      const response = isAdmin
        ? await getAdminVersionMonthStatus({
            ...period,
            user_id: userId as number,
          })
        : await getSelfVersionMonthStatus(period)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load billing version status.')
        )
      }
      return response.data
    },
    enabled: isAdmin ? userId != null : true,
    retry: false,
    refetchInterval: (query) => {
      const draftStatus = query.state.data?.active_draft?.status
      return isAdmin &&
        (draftStatus === 'queued' || draftStatus === 'generating')
        ? 5_000
        : false
    },
  })

  const refresh = () => {
    void queryClient.invalidateQueries({
      queryKey: ['billing-statement-version-month-status'],
    })
    props.onChanged()
  }

  const handleDraftCleaned = (draftPublicId: string) => {
    if (props.previewVersionId === draftPublicId) props.onPreviewChange(null)
    refresh()
  }

  const handleDownload = async (
    draftPublicId: string,
    role: string | undefined,
    admin: boolean
  ) => {
    const response = admin
      ? await getAdminVersionDownload(draftPublicId, role)
      : await getSelfVersionDownload(draftPublicId, role)
    if (!response.success || !response.data) {
      throw new Error(response.message || t('Download is not ready.'))
    }
    window.open(response.data.url, '_blank', 'noopener')
  }

  const downloadMutation = useMutation({
    mutationFn: (input: { draftPublicId: string; role?: string }) =>
      handleDownload(input.draftPublicId, input.role, isAdmin),
    onSuccess: () => toast.success(t('Download started.')),
    onError: (error: Error) => toast.error(error.message),
  })

  const status = statusQuery.data
  if (
    !status ||
    (!isAdmin &&
      !status.switch_enabled &&
      !status.current_version &&
      !status.active_draft)
  ) {
    // Customers retain the existing hidden state; administrators can review
    // sources while confirmation is disabled for a controlled rollout.
    return null
  }

  const monthEnded = isEndedMonth(period.end_timestamp)
  const current = status.current_version
  const draft = isAdmin ? status.active_draft : null

  return (
    <Card size='sm' className='border-dashed'>
      <CardContent className='flex flex-wrap items-center gap-2 py-3'>
        <VersionStatusBadge
          current={current}
          draftStatus={draft?.status}
          hasActiveDraft={status.has_active_draft}
          monthEnded={monthEnded}
          isAdmin={isAdmin}
        />
        {current?.version_number != null && (
          <span className='text-muted-foreground text-xs'>
            {t('Confirmed at {{time}}', {
              time: formatVersionTime(current.confirmed_at),
            })}
          </span>
        )}
        {current?.public_reason && (
          <span className='text-muted-foreground text-xs'>
            {t('Correction note')}: {current.public_reason}
          </span>
        )}
        {isAdmin && (
          <StatementSourceVerification
            key={`${userId}:${period.start_timestamp}:${period.end_timestamp}`}
            userId={userId as number}
            period={period}
            disabled={!status.topology_ok}
            onVerified={refresh}
          />
        )}
        {isAdmin && !status.topology_ok && (
          <span role='alert' className='text-destructive text-sm'>
            {t(
              'Statement generation requires billing records and account data in the same database.'
            )}
          </span>
        )}
        {status.retention_status === 'partial' && (
          <Badge variant='destructive'>
            {t('Source logs partially cleaned; correction is blocked.')}
          </Badge>
        )}
        {props.previewVersionId && (
          <Badge variant='secondary'>
            {draft?.draft_public_id === props.previewVersionId
              ? t('Previewing pending version')
              : t('Viewing historical version')}
          </Badge>
        )}
        <div className='ms-auto flex flex-wrap items-center gap-2'>
          {current && (
            <Button
              variant='outline'
              size='sm'
              disabled={downloadMutation.isPending}
              onClick={() =>
                downloadMutation.mutate({
                  draftPublicId: current.draft_public_id,
                  role: 'confirmation_note',
                })
              }
            >
              {t('Download confirmation note')}
            </Button>
          )}
          {isAdmin && monthEnded && !draft && !current && (
            <GenerateButton
              userId={userId as number}
              period={period}
              disabled={
                !status.switch_enabled ||
                !status.topology_ok ||
                !['intact', 'none'].includes(status.retention_status)
              }
              onDone={refresh}
            />
          )}
          {isAdmin && draft && isCleanableDraft(draft.status) && (
            <CleanupButton
              draftPublicId={draft.draft_public_id}
              onDone={() => handleDraftCleaned(draft.draft_public_id)}
            />
          )}
          {isAdmin && draft && isActiveDraft(draft.status) && (
            <>
              <Button
                variant='outline'
                size='sm'
                disabled={draft.status !== 'pending'}
                onClick={() =>
                  props.onPreviewChange(
                    props.previewVersionId === draft.draft_public_id
                      ? null
                      : draft.draft_public_id
                  )
                }
              >
                {props.previewVersionId === draft.draft_public_id
                  ? t('View customer current statement')
                  : t('Preview pending version')}
              </Button>
              {draft.status === 'pending' && (
                <Button
                  size='sm'
                  disabled={!status.switch_enabled}
                  onClick={() => setConfirmOpen(true)}
                >
                  {t('Confirm statement')}
                </Button>
              )}
              <AbandonButton
                draftPublicId={draft.draft_public_id}
                onDone={refresh}
              />
            </>
          )}
          {isAdmin && current && monthEnded && !draft && (
            <Button
              variant='outline'
              size='sm'
              disabled={
                !status.switch_enabled ||
                !status.topology_ok ||
                !['intact', 'none'].includes(status.retention_status)
              }
              onClick={() => setCorrectionOpen(true)}
            >
              {t('Start correction')}
            </Button>
          )}
          {current && (
            <Button
              variant='outline'
              size='sm'
              disabled={downloadMutation.isPending}
              onClick={() =>
                downloadMutation.mutate({
                  draftPublicId: current.draft_public_id,
                })
              }
            >
              {t('Download confirmed version')}
            </Button>
          )}
          <Button
            variant='ghost'
            size='sm'
            onClick={() => setHistoryOpen(true)}
          >
            {t('Version history ({{count}})', {
              count: status.versions.length,
            })}
          </Button>
        </div>
      </CardContent>
      {isAdmin && draft && props.previewVersionId === draft.draft_public_id && (
        <CardContent className='pt-0'>
          <VersionDiffSection
            userId={userId as number}
            period={period}
            draft={draft}
          />
        </CardContent>
      )}
      <VersionHistoryDrawer
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        versions={status.versions}
        currentDraftPublicId={current?.draft_public_id}
        isAdmin={isAdmin}
        onPreview={(draftPublicId) => {
          props.onPreviewChange(draftPublicId)
          setHistoryOpen(false)
        }}
        onCleaned={handleDraftCleaned}
        onDownload={(draftPublicId, role) =>
          downloadMutation.mutate({ draftPublicId, role })
        }
      />
      <ConfirmDialog
        key={draft?.draft_public_id ?? 'none'}
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        draft={draft}
        userId={userId as number}
        period={period}
        onConfirmed={refresh}
      />
      <CorrectionDialog
        open={correctionOpen}
        onOpenChange={setCorrectionOpen}
        userId={userId as number}
        period={period}
        onStarted={refresh}
      />
    </Card>
  )
}

function VersionStatusBadge(props: {
  current: BillingStatementVersionInfo | null
  draftStatus?: string
  hasActiveDraft: boolean
  monthEnded: boolean
  isAdmin: boolean
}) {
  const { t } = useTranslation()
  if (!props.monthEnded) {
    return <Badge variant='outline'>{t('Current month in progress')}</Badge>
  }
  if (props.current?.version_number != null) {
    return (
      <Badge variant='default'>
        {t('Confirmed v{{version}}', {
          version: props.current.version_number,
        })}
      </Badge>
    )
  }
  if (props.isAdmin && props.draftStatus) {
    return (
      <Badge variant='secondary'>
        {t(versionStatusLabel(props.draftStatus))}
      </Badge>
    )
  }
  if (props.hasActiveDraft) {
    return <Badge variant='secondary'>{t('Pending confirmation')}</Badge>
  }
  return (
    <Badge variant='outline'>
      {props.isAdmin
        ? t('Unconfirmed')
        : t('Unconfirmed; data may still change.')}
    </Badge>
  )
}

function GenerateButton(props: {
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  disabled: boolean
  onDone: () => void
}) {
  const { t } = useTranslation()
  const mutation = useMutation({
    mutationFn: () =>
      createAdminVersion({ ...props.period, user_id: props.userId }),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(
          response.message || t('Unable to generate statement version.')
        )
        return
      }
      toast.success(t('Statement version generation started.'))
      props.onDone()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  return (
    <Button
      size='sm'
      disabled={props.disabled || mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t('Generate pending version')}
    </Button>
  )
}

function AbandonButton(props: { draftPublicId: string; onDone: () => void }) {
  const { t } = useTranslation()
  const mutation = useMutation({
    mutationFn: () => abandonAdminVersion(props.draftPublicId),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to abandon draft.'))
        return
      }
      toast.success(t('Draft abandoned.'))
      props.onDone()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  return (
    <Button
      variant='ghost'
      size='sm'
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t('Abandon draft')}
    </Button>
  )
}

function CleanupButton(props: { draftPublicId: string; onDone: () => void }) {
  const { t } = useTranslation()
  const mutation = useMutation({
    mutationFn: () => cleanupAdminVersion(props.draftPublicId),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to clean up draft.'))
        return
      }
      toast.success(t('Draft cleaned up.'))
      props.onDone()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  return (
    <Button
      variant='outline'
      size='sm'
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t('Clean up draft')}
    </Button>
  )
}

function ConfirmDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  draft: BillingStatementVersionInfo | null
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  onConfirmed: () => void
}) {
  const { t } = useTranslation()
  const [acknowledged, setAcknowledged] = useState(false)
  const [publicReason, setPublicReason] = useState(
    props.draft?.public_reason ?? ''
  )
  const quality = props.draft?.data_quality
  const hasQualityNotes = !!quality && quality.status !== 'complete'
  const mutation = useMutation({
    mutationFn: () =>
      confirmAdminVersion(props.draft?.draft_public_id ?? '', {
        base_version_id: props.draft?.corrects_version_id,
        idempotency_key:
          typeof crypto !== 'undefined' && 'randomUUID' in crypto
            ? crypto.randomUUID()
            : `confirm-${Date.now()}`,
        acknowledged_quality: acknowledged
          ? JSON.stringify({
              acknowledged: true,
              draft_public_id: props.draft?.draft_public_id,
              status: quality?.status,
            })
          : '',
        public_reason: publicReason.trim(),
      }),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to confirm statement.'))
        return
      }
      toast.success(
        response.committed
          ? t('Statement version confirmed.')
          : t('Statement version already confirmed.')
      )
      props.onOpenChange(false)
      setAcknowledged(false)
      setPublicReason('')
      props.onConfirmed()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Confirm statement')}</DialogTitle>
          <DialogDescription>
            {t(
              'Confirmation publishes this version to the customer. Amounts cannot be edited afterwards; corrections require a new version.'
            )}
          </DialogDescription>
        </DialogHeader>
        {hasQualityNotes && (
          <div className='border-warning/40 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-sm'>
            <p>
              {t(
                'Some billing explanations are missing. Review these limitations before confirming the recorded statement.'
              )}
            </p>
            <ul className='mt-1 list-inside list-disc'>
              {qualityReasons(quality as BillingDataQuality, t).map(
                (message) => (
                  <li key={message}>{message}</li>
                )
              )}
            </ul>
            <label className='mt-2 flex items-center gap-2'>
              <Checkbox
                checked={acknowledged}
                onCheckedChange={(checked) => setAcknowledged(checked === true)}
              />
              <span>
                {t('I have reviewed the missing explanations above.')}
              </span>
            </label>
          </div>
        )}
        <div className='space-y-1.5'>
          <Label htmlFor='confirm-public-reason'>
            {t('Public reason (optional)')}
          </Label>
          <Input
            id='confirm-public-reason'
            value={publicReason}
            onChange={(event) => setPublicReason(event.target.value)}
            placeholder={t(
              'Shown to the customer, e.g. monthly statement confirmed'
            )}
          />
        </div>
        <DialogFooter>
          <Button
            disabled={
              mutation.isPending ||
              !quality ||
              (hasQualityNotes && !acknowledged)
            }
            onClick={() => mutation.mutate()}
          >
            {t('Confirm statement')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CorrectionDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  onStarted: () => void
}) {
  const { t } = useTranslation()
  const [publicReason, setPublicReason] = useState('')
  const [internalNote, setInternalNote] = useState('')
  const mutation = useMutation({
    mutationFn: () =>
      createAdminVersionCorrection(
        { ...props.period, user_id: props.userId },
        {
          public_reason: publicReason.trim(),
          internal_note: internalNote.trim() || undefined,
        }
      ),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to start correction.'))
        return
      }
      toast.success(t('Correction draft generation started.'))
      props.onOpenChange(false)
      setPublicReason('')
      setInternalNote('')
      props.onStarted()
    },
    onError: (error: Error) => toast.error(error.message),
  })
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Start correction')}</DialogTitle>
          <DialogDescription>
            {t(
              'A correction draft is generated from current sources. The confirmed version stays visible until the correction is confirmed.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-1.5'>
          <Label htmlFor='correction-public-reason'>
            {t('Customer-visible reason')}
          </Label>
          <Input
            id='correction-public-reason'
            value={publicReason}
            onChange={(event) => setPublicReason(event.target.value)}
            placeholder={t('e.g. Added verified missing records')}
          />
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor='correction-internal-note'>
            {t('Internal note (optional)')}
          </Label>
          <Input
            id='correction-internal-note'
            value={internalNote}
            onChange={(event) => setInternalNote(event.target.value)}
          />
        </div>
        <DialogFooter>
          <Button
            disabled={mutation.isPending || publicReason.trim() === ''}
            onClick={() => mutation.mutate()}
          >
            {t('Generate correction draft')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function VersionHistoryDrawer(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  versions: BillingStatementVersionInfo[]
  currentDraftPublicId: string | undefined
  isAdmin: boolean
  onPreview: (draftPublicId: string) => void
  onDownload: (draftPublicId: string, role?: string) => void
  onCleaned: (draftPublicId: string) => void
}) {
  const { t } = useTranslation()
  return (
    <Drawer open={props.open} onOpenChange={props.onOpenChange}>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{t('Version history')}</DrawerTitle>
          <DrawerDescription>
            {props.isAdmin
              ? t(
                  'All versions including drafts. Customers only see confirmed versions.'
                )
              : t('Confirmed versions of this billing period.')}
          </DrawerDescription>
        </DrawerHeader>
        <div className='overflow-y-auto px-4 pb-4'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Version')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Confirmed at')}</TableHead>
                <TableHead>{t('Note')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {[...props.versions]
                .sort(
                  (a, b) => (b.version_number ?? 0) - (a.version_number ?? 0)
                )
                .map((version) => (
                  <TableRow key={version.draft_public_id}>
                    <TableCell>
                      {version.version_number != null
                        ? `v${version.version_number}`
                        : t('Draft')}
                      {version.draft_public_id ===
                        props.currentDraftPublicId && (
                        <Badge variant='secondary' className='ms-2'>
                          {t('Current')}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {t(versionStatusLabel(version.status))}
                    </TableCell>
                    <TableCell>
                      {version.confirmed_at
                        ? formatVersionTime(version.confirmed_at)
                        : '—'}
                    </TableCell>
                    <TableCell className='max-w-48 truncate'>
                      {version.public_reason || '—'}
                    </TableCell>
                    <TableCell className='text-right'>
                      <div className='flex justify-end gap-1'>
                        {props.isAdmin && isCleanableDraft(version.status) && (
                          <CleanupButton
                            draftPublicId={version.draft_public_id}
                            onDone={() =>
                              props.onCleaned(version.draft_public_id)
                            }
                          />
                        )}
                        {version.status === 'confirmed' && (
                          <>
                            <Button
                              variant='link'
                              size='xs'
                              onClick={() =>
                                props.onPreview(version.draft_public_id)
                              }
                            >
                              {t('View')}
                            </Button>
                            <Button
                              variant='link'
                              size='xs'
                              onClick={() =>
                                props.onDownload(version.draft_public_id)
                              }
                            >
                              {t('Download')}
                            </Button>
                            <Button
                              variant='link'
                              size='xs'
                              onClick={() =>
                                props.onDownload(
                                  version.draft_public_id,
                                  'confirmation_note'
                                )
                              }
                            >
                              {t('Download confirmation note')}
                            </Button>
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        </div>
      </DrawerContent>
    </Drawer>
  )
}

// VersionDiffSection 更正草稿与基准版的差异对比（方案 6.1）。
function VersionDiffSection(props: {
  userId: number
  period: { start_timestamp: number; end_timestamp: number }
  draft: BillingStatementVersionInfo
}) {
  const { t } = useTranslation()
  const diffQuery = useQuery({
    queryKey: [
      'billing-statement-version-diff',
      props.userId,
      props.period.start_timestamp,
      props.draft.id,
    ],
    queryFn: async () => {
      const response = await getAdminVersionDiff({
        ...props.period,
        user_id: props.userId,
        compare_id: props.draft.id,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Unable to load version diff.'))
      }
      return response.data.diff
    },
    enabled: props.draft.corrects_version_id != null,
    retry: false,
  })
  if (props.draft.corrects_version_id == null) {
    return null
  }
  const diff = diffQuery.data
  if (diffQuery.isPending) {
    return <p className='text-muted-foreground text-xs'>{t('Loading diff…')}</p>
  }
  if (!diff) {
    return null
  }
  return (
    <div className='space-y-2'>
      <div className='text-sm font-medium'>
        {t('Changes from confirmed version')}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Item')}</TableHead>
            <TableHead className='text-right'>{t('Confirmed')}</TableHead>
            <TableHead className='text-right'>
              {t('Correction draft')}
            </TableHead>
            <TableHead className='text-right'>{t('Difference')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {diffItemRows(diff, t).map((row) => (
            <TableRow key={row.key}>
              <TableCell>{row.label}</TableCell>
              <TableCell className='text-right tabular-nums'>
                {row.base ?? t('Unavailable')}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {row.compare ?? t('Unavailable')}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {row.delta != null ? formatDelta(row.delta) : '—'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <StatementVersionDiffDetails diff={diff} />
      {diff.models.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Billing mode')}</TableHead>
              <TableHead className='text-right'>{t('Difference')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {diff.models
              .filter(
                (row) =>
                  row.net.delta_usd != null &&
                  !/^[-+]?0\.0+$/.test(row.net.delta_usd)
              )
              .map((row) => (
                <TableRow
                  key={`${row.group_id}-${row.model_name}-${row.billing_mode}`}
                >
                  <TableCell>{row.model_name || t('Unknown model')}</TableCell>
                  <TableCell>{row.billing_mode}</TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatDelta(row.net.delta_usd as string)}
                  </TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

function diffItemRows(
  diff: BillingStatementVersionDiff,
  t: (key: string) => string
) {
  const labels: Record<string, string> = {
    gross_quota: t('Consumption amount'),
    refund_quota: t('Refund amount'),
    net_quota: t('Net amount'),
    original_quota: t('Estimated list price'),
    discount_quota: t('Estimated savings'),
    requests: t('Requests'),
  }
  return Object.entries(labels).map(([key, label]) => {
    const item = diff.items[key]
    return {
      key,
      label,
      base: key === 'requests' ? item?.base : diffUSD(item?.base_usd),
      compare: key === 'requests' ? item?.compare : diffUSD(item?.compare_usd),
      delta: key === 'requests' ? item?.delta : item?.delta_usd,
    }
  })
}

function formatDelta(delta: number | string) {
  if (typeof delta === 'number') return delta > 0 ? `+${delta}` : String(delta)
  return delta.startsWith('-') ? `-$${delta.slice(1)}` : `$${delta}`
}

function isActiveDraft(status: string) {
  return status === 'queued' || status === 'generating' || status === 'pending'
}

function isCleanableDraft(status: string) {
  return (
    status === 'cleaning' ||
    status === 'invalid' ||
    status === 'failed' ||
    status === 'cancelled'
  )
}

function versionStatusLabel(status: string) {
  switch (status) {
    case 'confirmed':
      return 'Confirmed'
    case 'pending':
      return 'Pending confirmation'
    case 'generating':
      return 'Generating'
    case 'queued':
      return 'Queued'
    case 'invalid':
      return 'Invalid: source changed'
    case 'failed':
      return 'Generation failed'
    case 'cancelled':
      return 'Cancelled'
    case 'cleaning':
      return 'Cleaning...'
    default:
      return status
  }
}

function qualityReasons(
  quality: BillingDataQuality,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const reasons: string[] = []
  if ((quality.input_tokens_unavailable_requests ?? 0) > 0) {
    reasons.push(
      t('Input token totals unavailable: {{count}} records', {
        count: quality.input_tokens_unavailable_requests,
      })
    )
  }
  if ((quality.unknown_billing_mode_requests ?? 0) > 0) {
    reasons.push(
      t('Unknown billing mode: {{count}} records', {
        count: quality.unknown_billing_mode_requests,
      })
    )
  }
  if ((quality.unavailable_requests ?? 0) > 0) {
    reasons.push(
      t('Usage metadata unreadable: {{count}} records', {
        count: quality.unavailable_requests,
      })
    )
  }
  if ((quality.cache_write_unavailable_requests ?? 0) > 0) {
    reasons.push(
      t('Cache write usage unavailable: {{count}} records', {
        count: quality.cache_write_unavailable_requests,
      })
    )
  }
  if ((quality.missing_historical_price_rows ?? 0) > 0) {
    reasons.push(
      t('Historical prices unavailable: {{count}} records', {
        count: quality.missing_historical_price_rows,
      })
    )
  }
  if ((quality.provider_model_fallback_rows ?? 0) > 0) {
    reasons.push(
      t('Upstream model identity missing: {{count}} records', {
        count: quality.provider_model_fallback_rows,
      })
    )
  }
  return reasons
}

// isEndedMonth 判断账期是否为已结束自然月（Asia/Shanghai，end_timestamp 为包含式月末）。
function isEndedMonth(endTimestamp: number) {
  return endTimestamp + 1 <= shanghaiCurrentMonthStart()
}

function shanghaiCurrentMonthStart() {
  const now = new Date()
  const shanghai = new Date(now.getTime() + 8 * 3600 * 1000)
  return (
    (Date.UTC(shanghai.getUTCFullYear(), shanghai.getUTCMonth(), 1) -
      8 * 3600 * 1000) /
    1000
  )
}

function formatVersionTime(timestamp: number) {
  return new Date(timestamp * 1000).toLocaleString()
}

function diffUSD(value: string | undefined) {
  return value == null ? null : `$${value}`
}
