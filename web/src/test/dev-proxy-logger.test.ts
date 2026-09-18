import { describe, expect, it, vi } from 'vitest'

import { createDevProxyLogger } from '../../scripts/dev-proxy-logger'

describe('development proxy diagnostics', () => {
  it('keeps the error code without credentials or request URLs', () => {
    const output = vi.spyOn(console, 'error').mockImplementation(() => {})
    const logger = createDevProxyLogger(
      'http://user:secret@localhost:3501/private?key=secret',
      '/api'
    )
    logger.error(
      '[HPM] %s %s [%s] (%s)',
      '/api/token/secret?token=secret',
      'http://secret/',
      'ECONNREFUSED',
      'documentation'
    )
    expect(output).toHaveBeenCalledWith(
      '[dev-proxy] /api -> http://localhost:3501: ECONNREFUSED'
    )
    expect(output.mock.calls.flat().join('')).not.toContain('secret')
  })

  it('never prints arbitrary exception messages', () => {
    const output = vi.spyOn(console, 'error').mockImplementation(() => {})
    const logger = createDevProxyLogger('http://localhost:3501', '/v1')
    logger.error(
      Object.assign(new Error('Authorization: secret'), { code: 'ETIMEDOUT' })
    )
    expect(output).toHaveBeenLastCalledWith(
      '[dev-proxy] /v1 -> http://localhost:3501: ETIMEDOUT'
    )
    logger.error(new Error('Cookie: secret'))
    expect(output).toHaveBeenLastCalledWith(
      '[dev-proxy] /v1 -> http://localhost:3501: UNKNOWN_PROXY_ERROR'
    )
  })
})
