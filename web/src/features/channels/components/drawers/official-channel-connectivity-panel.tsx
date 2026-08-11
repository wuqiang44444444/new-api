import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { type Control, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import {
  deleteChannelAssetCredential,
  testChannelAssetAction,
  testChannelVideoAPI,
} from '../../api'
import { channelsQueryKeys } from '../../lib/channel-actions'
import type { ChannelFormValues } from '../../lib/channel-form'
import {
  getOfficialConnectivityAvailability,
  getOfficialConnectivityMessage,
} from '../../lib/official-channel-connectivity'

type TestStatus = 'idle' | 'testing' | 'success' | 'error'

type ConnectivityResult = {
  status: TestStatus
  message?: string
  seconds?: number
}

interface OfficialChannelConnectivityPanelProps {
  channelId: number
  control: Control<ChannelFormValues>
  credentialConfigured: boolean
  savedAssetProtocol?: string
  savedVideoProtocol?: string
  sensitiveLocked: boolean
  onCredentialCleared: () => void
}

function ConnectivityStatusBadge({ status }: { status: TestStatus }) {
  const { t } = useTranslation()
  if (status === 'testing') {
    return (
      <StatusBadge label={t('Testing...')} variant='info' copyable={false} />
    )
  }
  if (status === 'success') {
    return (
      <StatusBadge label={t('Available')} variant='success' copyable={false} />
    )
  }
  if (status === 'error') {
    return <StatusBadge label={t('Failed')} variant='danger' copyable={false} />
  }
  return (
    <StatusBadge label={t('Not tested')} variant='neutral' copyable={false} />
  )
}

function ConnectivityTestCard(props: {
  title: string
  description: string
  buttonLabel: string
  pendingLabel: string
  disabled: boolean
  result: ConnectivityResult
  onTest: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 bg-background space-y-3 rounded-md border p-3'>
      <div className='flex items-center justify-between gap-3'>
        <span className='text-sm font-medium'>{props.title}</span>
        <ConnectivityStatusBadge status={props.result.status} />
      </div>
      <p className='text-muted-foreground text-xs'>{props.description}</p>
      {props.result.message ? (
        <p
          className={
            props.result.status === 'error'
              ? 'text-destructive text-xs'
              : 'text-muted-foreground text-xs'
          }
        >
          {t(props.result.message)}
        </p>
      ) : null}
      {props.result.status !== 'idle' &&
      props.result.status !== 'testing' &&
      typeof props.result.seconds === 'number' ? (
        <p className='text-muted-foreground text-xs'>
          {t('{{seconds}} s', {
            seconds: props.result.seconds.toFixed(2),
          })}
        </p>
      ) : null}
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={props.disabled || props.result.status === 'testing'}
        onClick={props.onTest}
      >
        {props.result.status === 'testing'
          ? props.pendingLabel
          : props.buttonLabel}
      </Button>
    </div>
  )
}

export function OfficialChannelConnectivityPanel(
  props: OfficialChannelConnectivityPanelProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [videoResult, setVideoResult] = useState<ConnectivityResult>({
    status: 'idle',
  })
  const [assetResult, setAssetResult] = useState<ConnectivityResult>({
    status: 'idle',
  })
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)
  const [isClearing, setIsClearing] = useState(false)
  const [clearedChannelId, setClearedChannelId] = useState<number | null>(null)
  const [
    assetProtocol,
    videoProtocol,
    pendingVideoKey,
    pendingAccessKeyID,
    pendingSecretAccessKey,
  ] = useWatch({
    control: props.control,
    name: [
      'asset_upstream_protocol',
      'video_upstream_protocol',
      'key',
      'asset_access_key_id',
      'asset_secret_access_key',
    ],
  })

  const credentialConfigured =
    props.credentialConfigured && clearedChannelId !== props.channelId
  const isRelevant =
    (assetProtocol !== undefined && assetProtocol !== 'none') ||
    (props.savedAssetProtocol !== undefined &&
      props.savedAssetProtocol !== 'none') ||
    credentialConfigured
  if (!isRelevant) return null

  const hasPendingVideoKey = Boolean(pendingVideoKey?.trim())
  const hasPendingAssetCredential = Boolean(
    pendingAccessKeyID?.trim() || pendingSecretAccessKey?.trim()
  )
  const availability = getOfficialConnectivityAvailability({
    assetProtocol,
    savedAssetProtocol: props.savedAssetProtocol,
    videoProtocol,
    savedVideoProtocol: props.savedVideoProtocol,
    credentialConfigured,
    hasPendingVideoKey,
    hasPendingAssetCredential,
    sensitiveLocked: props.sensitiveLocked,
  })
  const showVideoTest =
    videoProtocol === 'modelark_v3_volcengine' ||
    videoProtocol === 'modelark_v3_byteplus' ||
    props.savedVideoProtocol === 'modelark_v3_volcengine' ||
    props.savedVideoProtocol === 'modelark_v3_byteplus'
  const officialAssetAction =
    assetProtocol === 'volcengine_assets_action_v2024_01_01' ||
    assetProtocol === 'byteplus_assets_action_v2024_01_01' ||
    props.savedAssetProtocol === 'volcengine_assets_action_v2024_01_01' ||
    props.savedAssetProtocol === 'byteplus_assets_action_v2024_01_01'

  const testVideo = async () => {
    setVideoResult({ status: 'testing' })
    try {
      const response = await testChannelVideoAPI(props.channelId)
      if (response.success) {
        setVideoResult({ status: 'success', seconds: response.time })
        return
      }
      setVideoResult({
        status: 'error',
        message: getOfficialConnectivityMessage(
          response,
          'Video API test failed.'
        ),
        seconds: response.time,
      })
    } catch {
      setVideoResult({ status: 'error', message: 'Video API test failed.' })
    }
  }

  const testAsset = async () => {
    setAssetResult({ status: 'testing' })
    try {
      const response = await testChannelAssetAction(props.channelId)
      if (response.success) {
        setAssetResult({ status: 'success', seconds: response.time })
        return
      }
      setAssetResult({
        status: 'error',
        message: getOfficialConnectivityMessage(
          response,
          officialAssetAction
            ? 'Asset Action test failed.'
            : 'Asset library test failed.'
        ),
        seconds: response.time,
      })
    } catch {
      setAssetResult({
        status: 'error',
        message: officialAssetAction
          ? 'Asset Action test failed.'
          : 'Asset library test failed.',
      })
    }
  }

  const clearCredential = async () => {
    setIsClearing(true)
    try {
      const response = await deleteChannelAssetCredential(props.channelId)
      if (!response.success) {
        toast.error(t('Failed to clear asset credentials'), {
          description: response.message
            ? t(response.message)
            : t('Try again after checking the channel configuration.'),
        })
        return
      }
      setClearedChannelId(props.channelId)
      setAssetResult({ status: 'idle' })
      props.onCredentialCleared()
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(props.channelId),
      })
      toast.success(t('Asset credentials cleared'))
      setClearConfirmOpen(false)
    } catch (error) {
      let description = t('Try again after checking the channel configuration.')
      if (error && typeof error === 'object' && 'response' in error) {
        const response = (
          error as {
            response?: {
              data?: { error_code?: unknown; message?: unknown }
            }
          }
        ).response?.data
        if (response) {
          description = t(
            getOfficialConnectivityMessage(
              {
                success: false,
                error_code:
                  typeof response.error_code === 'string'
                    ? response.error_code
                    : undefined,
                message:
                  typeof response.message === 'string'
                    ? response.message
                    : undefined,
              },
              'Try again after checking the channel configuration.'
            )
          )
        }
      }
      toast.error(t('Failed to clear asset credentials'), { description })
    } finally {
      setIsClearing(false)
    }
  }

  return (
    <>
      <div className='border-border/60 bg-muted/10 space-y-3 rounded-lg border p-4'>
        <div>
          <h4 className='text-sm font-medium'>{t('Connectivity tests')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {showVideoTest
              ? t(
                  'Video and asset credentials are tested separately with read-only requests.'
                )
              : t(
                  'The saved asset channel is tested with a read-only list request.'
                )}
          </p>
        </div>
        {availability.hasUnsavedTestChanges ? (
          <p className='text-warning text-xs'>
            {t('Save the pending channel changes before testing.')}
          </p>
        ) : null}
        <div className='grid gap-3 sm:grid-cols-2'>
          {showVideoTest ? (
            <ConnectivityTestCard
              title={t('Video API')}
              description={t(
                'Lists at most one existing video task and never creates or deletes a task.'
              )}
              buttonLabel={t('Test Video API')}
              pendingLabel={t('Testing Video API...')}
              disabled={!availability.videoCanTest}
              result={videoResult}
              onTest={testVideo}
            />
          ) : null}
          <ConnectivityTestCard
            title={officialAssetAction ? t('Asset Action') : t('Asset Library')}
            description={t(
              'Lists at most one asset and never creates or deletes an upstream resource.'
            )}
            buttonLabel={
              officialAssetAction
                ? t('Test Asset Action')
                : t('Test Asset Library')
            }
            pendingLabel={
              officialAssetAction
                ? t('Testing Asset Action...')
                : t('Testing Asset Library...')
            }
            disabled={!availability.assetCanTest}
            result={assetResult}
            onTest={testAsset}
          />
        </div>
        {credentialConfigured ? (
          <div className='border-border/60 flex flex-col gap-2 border-t pt-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <p className='text-sm font-medium'>
                {t('Stored asset credentials')}
              </p>
              {!availability.canClearCredential ? (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'Disable Official Action Assets and save the channel before clearing its stored credentials.'
                  )}
                </p>
              ) : null}
            </div>
            <Button
              type='button'
              size='sm'
              variant='destructive'
              disabled={!availability.canClearCredential}
              onClick={() => setClearConfirmOpen(true)}
            >
              {t('Clear Asset Credentials')}
            </Button>
          </div>
        ) : null}
      </div>

      <ConfirmDialog
        open={clearConfirmOpen}
        onOpenChange={setClearConfirmOpen}
        title={t('Clear Asset Credentials')}
        desc={t(
          'Clear the stored Access Key ID and Secret Access Key? This action cannot be undone.'
        )}
        confirmText={t('Clear credentials')}
        destructive
        isLoading={isClearing}
        handleConfirm={clearCredential}
      />
    </>
  )
}
