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
  page: number
) {
  const result = await api.get<
    Envelope<{ items: EvidenceIndex[]; total: number }>
  >('/api/task_request_evidence', {
    params: { ...filter, p: page, page_size: 20 },
  })
  if (!result.data.success) throw new Error('Evidence query failed')
  return result.data.data
}
export async function getEvidenceDetail(id: number) {
  const result = await api.get<
    Envelope<{ evidence: EvidenceIndex; events: EvidenceEvent[] }>
  >(`/api/task_request_evidence/${id}`)
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

// Read recorded bodies rather than reconstructing requests from current settings.
export async function getTaskRequestBodies(
  taskId: string,
  stage: 'north_receive' | 'southbound_send'
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
    const list = await getEvidenceList({ task_id: taskId }, page)
    total = list.total
    for (const item of list.items) {
      const detail = await getEvidenceDetail(item.id)
      for (const event of detail.events.filter(
        (event) => event.stage === stage
      )) {
        let text: string | null = null
        let bodyStatus = event.body_status ?? 'available'
        if (detail.evidence.body_expired) bodyStatus = 'expired'
        else if (!event.has_body) bodyStatus = 'not_recorded'
        if (bodyStatus === 'available') {
          try {
            const response = await api.get<string>(
              `/api/task_request_evidence/${item.id}/events/${event.id}/object`,
              {
                responseType: 'text',
                transformResponse: [(value: string) => value],
                skipErrorHandler: true,
              }
            )
            text = response.data
          } catch (error) {
            if (
              !isAxiosError(error) ||
              error.response?.status === 401 ||
              error.response?.status === 403
            ) {
              throw error
            }
            bodyStatus = 'read_failed'
            // The object may disappear after the detail request. Preserve the
            // server's safe category, never display its raw response as a body.
            if (
              error.response?.status === 410 ||
              error.response?.status === 503
            ) {
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
          }
        }
        bodies.push({
          requestId: item.request_id,
          eventId: event.id,
          complete: event.complete,
          bodyStatus,
          text,
        })
      }
    }
    page++
  } while ((page - 1) * 20 < total)
  return bodies
}
