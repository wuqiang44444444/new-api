import { isAxiosError } from 'axios'

import { api } from '@/lib/api'

export interface EvidenceIndex {
  id: number
  request_id: string
  body_expired: boolean
}
export interface EvidenceEvent {
  id: number
  stage: string
  phase: string
  complete: boolean
  has_body: boolean
  body_status?: string
  preview: string
  byte_count: number
  status_code: number
}
interface Envelope<T> {
  success: boolean
  data: T
}
export async function getEvidenceList(
  filter: { task_id?: string; request_id?: string },
  page: number,
  signal?: AbortSignal
) {
  const result = await api.get<
    Envelope<{ items: EvidenceIndex[]; total: number }>
  >('/api/task_request_evidence', {
    params: { ...filter, p: page, page_size: 20 },
    // Abortable reads belong to dialogs that render their own error/retry UI.
    ...(signal
      ? { signal, disableDuplicate: true, skipErrorHandler: true }
      : {}),
  })
  signal?.throwIfAborted()
  if (!result.data.success) throw new Error('Evidence query failed')
  return result.data.data
}
export async function getEvidenceDetail(id: number, signal?: AbortSignal) {
  const result = await api.get<
    Envelope<{ evidence: EvidenceIndex; events: EvidenceEvent[] }>
  >(
    `/api/task_request_evidence/${id}`,
    signal
      ? { signal, disableDuplicate: true, skipErrorHandler: true }
      : undefined
  )
  signal?.throwIfAborted()
  if (!result.data.success) throw new Error('Evidence query failed')
  return result.data.data
}
export async function downloadEvidence(id: number, eventId: number) {
  const response = await api.get<Blob>(
    `/api/task_request_evidence/${id}/events/${eventId}/object`,
    { responseType: 'blob' }
  )
  const url = URL.createObjectURL(response.data)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `evidence-${eventId}.bin`
  anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// Read the recorded original through the Root-only view endpoint. The server
// owns readability classification; auth failures propagate as real errors.
export async function readEvidenceOriginalText(
  id: number,
  eventId: number,
  signal?: AbortSignal
): Promise<{ text: string | null; bodyStatus: string }> {
  signal?.throwIfAborted()
  try {
    const response = await api.get<string>(
      `/api/task_request_evidence/${id}/events/${eventId}/content`,
      {
        responseType: 'text',
        transformResponse: [(value: string) => value],
        skipErrorHandler: true,
        signal,
        disableDuplicate: true,
      }
    )
    signal?.throwIfAborted()
    return { text: response.data, bodyStatus: 'available' }
  } catch (error) {
    signal?.throwIfAborted()
    if (
      !isAxiosError(error) ||
      error.response?.status === 401 ||
      error.response?.status === 403
    ) {
      throw error
    }
    let bodyStatus = 'read_failed'
    // The object may disappear after the detail request. Preserve the
    // server's safe category, never display its raw response as a body.
    if (error.response?.status === 410 || error.response?.status === 503) {
      try {
        const failure: { body_status?: unknown } | null = JSON.parse(
          error.response.data
        )
        if (typeof failure?.body_status === 'string') {
          bodyStatus = failure.body_status
        }
      } catch {
        // A proxy may return HTML; it is still a per-object read failure.
      }
    }
    return { text: null, bodyStatus }
  }
}

// Read recorded bodies rather than reconstructing requests from current settings.
export async function getTaskRequestBodies(
  taskId: string,
  stage: 'north_receive' | 'southbound_send',
  signal?: AbortSignal
) {
  const bodies: {
    requestId: string
    eventId: number
    complete: boolean
    bodyStatus: string
    text: string | null
  }[] = []
  let page = 1
  let total = 0
  do {
    const list = await getEvidenceList({ task_id: taskId }, page, signal)
    total = list.total
    for (const item of list.items) {
      const detail = await getEvidenceDetail(item.id, signal)
      for (const event of detail.events.filter(
        (event) => event.stage === stage
      )) {
        let bodyStatus = event.body_status ?? 'available'
        if (detail.evidence.body_expired) bodyStatus = 'expired'
        else if (!event.has_body) bodyStatus = 'not_recorded'
        const { text, bodyStatus: readStatus } =
          bodyStatus === 'available'
            ? await readEvidenceOriginalText(item.id, event.id, signal)
            : { text: null, bodyStatus }
        bodies.push({
          requestId: item.request_id,
          eventId: event.id,
          complete: event.complete,
          bodyStatus: readStatus,
          text,
        })
      }
    }
    page++
  } while ((page - 1) * 20 < total)
  return bodies
}
