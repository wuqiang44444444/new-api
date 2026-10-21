import { isAxiosError } from 'axios'

import { api } from '@/lib/api'

export interface EvidenceIndex {
  id: number
  request_id: string
  body_expired: boolean
}
export interface EvidenceEvent {
  id: number
  seq: number
  name: string
  status_code: number
  method?: string
  target?: string
  content_type?: string
  created_at?: number
}
export interface EvidenceContent {
  text: string | null
  bodyStatus: string
  meta?: Record<string, unknown> | null
  base64?: string
  content_type?: string
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

// The directory contains locations only. A selected object is read separately.
export async function readEvidenceOriginalText(
  id: number,
  eventId: number,
  signal?: AbortSignal
): Promise<EvidenceContent> {
  signal?.throwIfAborted()
  try {
    const response = await api.get<
      Envelope<{
        text: string | null
        body_status: string
        meta?: Record<string, unknown> | null
        base64?: string
        content_type?: string
      }>
    >(`/api/task_request_evidence/${id}/events/${eventId}/content`, {
      skipErrorHandler: true,
      signal,
      disableDuplicate: true,
    })
    signal?.throwIfAborted()
    if (!response.data.success) throw new Error('Evidence query failed')
    const content = response.data.data
    return { ...content, bodyStatus: content.body_status }
  } catch (error) {
    signal?.throwIfAborted()
    if (
      !isAxiosError(error) ||
      error.response?.status === 401 ||
      error.response?.status === 403
    ) {
      throw error
    }
    const failure = error.response?.data as
      | { body_status?: unknown }
      | undefined
    return {
      text: null,
      bodyStatus:
        typeof failure?.body_status === 'string'
          ? failure.body_status
          : 'read_failed',
    }
  }
}

// Request shortcuts fetch directories, never all the captured bodies.
export async function getTaskRequestEntries(
  taskId: string,
  name: string,
  signal?: AbortSignal
) {
  const entries: {
    evidenceId: number
    eventId: number
    requestId: string
    expired: boolean
  }[] = []
  let page = 1
  let total = 0
  do {
    const list = await getEvidenceList({ task_id: taskId }, page, signal)
    total = list.total
    const details = await Promise.all(
      list.items.map((item) => getEvidenceDetail(item.id, signal))
    )
    for (const [index, detail] of details.entries()) {
      for (const event of detail.events) {
        if (event.name === name) {
          entries.push({
            evidenceId: list.items[index].id,
            eventId: event.id,
            requestId: list.items[index].request_id,
            expired: detail.evidence.body_expired,
          })
        }
      }
    }
    page++
  } while ((page - 1) * 20 < total)
  return entries
}
