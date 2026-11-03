import { isAxiosError } from 'axios'

import { api, getFreshAuthHeaders } from '@/lib/api'

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
export interface EvidenceMessage {
  offset: number
  message_type: number
  created_at_ms: number
  original_bytes: number
  error?: string
  text: string | null
  base64?: string
  display_limited?: boolean
  formatted_text?: string | null
  json_status?: string
}
export interface EvidenceContent {
  text: string | null
  bodyStatus: string
  meta?: Record<string, unknown> | null
  base64?: string
  content_type?: string
  display_limited?: boolean
  formatted_text?: string | null
  json_status?: string
  messages?: EvidenceMessage[]
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
interface EvidenceSaveHandle {
  createWritable(): Promise<WritableStream<Uint8Array>>
}

export async function downloadEvidence(id: number, eventId: number) {
  const picker = (
    window as Window & {
      showSaveFilePicker?: (options: {
        suggestedName: string
      }) => Promise<EvidenceSaveHandle>
    }
  ).showSaveFilePicker
  const filename = `evidence-${eventId}.bin`
  const path = `/api/task_request_evidence/${id}/events/${eventId}/object`
  if (picker) {
    let handle: EvidenceSaveHandle
    try {
      // Invoke while the user's download gesture is still active.
      handle = await picker.call(window, { suggestedName: filename })
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') return
      throw error
    }
    const headers = await getFreshAuthHeaders()
    const url = new URL(api.getUri({ url: path }), window.location.href)
    const response = await fetch(url, { headers, cache: 'no-store' })
    if (!response.ok || !response.body) {
      throw new Error('Evidence download failed')
    }
    await response.body.pipeTo(await handle.createWritable())
    return
  }
  // Browsers without a streaming file API retain the existing Blob download.
  // Server decryption stays streaming; only an explicitly selected download is loaded.
  const response = await api.get<Blob>(path, { responseType: 'blob' })
  const url = URL.createObjectURL(response.data)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// The directory contains locations only. A selected object is read separately.
export async function readEvidenceOriginalText(
  id: number,
  eventId: number,
  signal?: AbortSignal,
  formatJSON = false
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
        display_limited?: boolean
        formatted_text?: string | null
        json_status?: string
        messages?: EvidenceMessage[]
      }>
    >(`/api/task_request_evidence/${id}/events/${eventId}/content`, {
      ...(formatJSON ? { params: { format: 'json' } } : {}),
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
