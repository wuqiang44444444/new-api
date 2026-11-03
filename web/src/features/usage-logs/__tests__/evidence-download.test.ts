import { afterEach, describe, expect, it, vi } from 'vitest'

import { downloadEvidence } from '../evidence-api'

const mocks = vi.hoisted(() => ({
  freshHeaders: vi.fn(),
  getUri: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: { getUri: mocks.getUri },
  getFreshAuthHeaders: mocks.freshHeaders,
}))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
  Reflect.deleteProperty(window, 'showSaveFilePicker')
})

describe('streaming evidence download', () => {
  it('saves authenticated bytes through the stream using the configured API URL', async () => {
    const saved: Uint8Array[] = []
    const writable = new WritableStream<Uint8Array>({
      write: (chunk) => {
        saved.push(chunk)
      },
    })
    const createWritable = vi.fn().mockResolvedValue(writable)
    const picker = vi.fn().mockResolvedValue({ createWritable })
    Object.defineProperty(window, 'showSaveFilePicker', {
      configurable: true,
      value: picker,
    })
    mocks.freshHeaders.mockResolvedValue({ Authorization: 'Bearer fixture' })
    mocks.getUri.mockReturnValue('/gateway/api/evidence/object')
    const fetcher = vi.fn().mockResolvedValue(new Response('opaque evidence'))
    vi.stubGlobal('fetch', fetcher)
    await downloadEvidence(7, 8)
    expect(picker).toHaveBeenCalledWith({ suggestedName: 'evidence-8.bin' })
    expect(mocks.getUri).toHaveBeenCalledWith({
      url: '/api/task_request_evidence/7/events/8/object',
    })
    expect(fetcher).toHaveBeenCalledWith(
      new URL('/gateway/api/evidence/object', window.location.href),
      { headers: { Authorization: 'Bearer fixture' }, cache: 'no-store' }
    )
    expect(new TextDecoder().decode(saved[0])).toBe('opaque evidence')
    expect(createWritable).toHaveBeenCalledOnce()
  })

  it('does not authenticate or fetch when the file selection is cancelled', async () => {
    Object.defineProperty(window, 'showSaveFilePicker', {
      configurable: true,
      value: vi
        .fn()
        .mockRejectedValue(new DOMException('cancelled', 'AbortError')),
    })
    const fetcher = vi.fn()
    vi.stubGlobal('fetch', fetcher)
    await expect(downloadEvidence(7, 8)).resolves.toBeUndefined()
    expect(mocks.freshHeaders).not.toHaveBeenCalled()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('does not open or truncate a destination file for an unauthorized response', async () => {
    const createWritable = vi.fn()
    Object.defineProperty(window, 'showSaveFilePicker', {
      configurable: true,
      value: vi.fn().mockResolvedValue({ createWritable }),
    })
    mocks.freshHeaders.mockResolvedValue({})
    mocks.getUri.mockReturnValue('/api/evidence/object')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('denied', { status: 403 }))
    )
    await expect(downloadEvidence(7, 8)).rejects.toThrow(
      'Evidence download failed'
    )
    expect(createWritable).not.toHaveBeenCalled()
  })
})
