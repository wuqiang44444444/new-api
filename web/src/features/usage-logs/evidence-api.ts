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
    expired: boolean
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
        if (event.has_body && !detail.evidence.body_expired) {
          const response = await api.get<string>(
            `/api/task_request_evidence/${item.id}/events/${event.id}/object`,
            {
              responseType: 'text',
              transformResponse: [(value: string) => value],
            }
          )
          text = response.data
        }
        bodies.push({
          requestId: item.request_id,
          eventId: event.id,
          complete: event.complete,
          expired: detail.evidence.body_expired,
          text,
        })
      }
    }
    page++
  } while ((page - 1) * 20 < total)
  return bodies
}
