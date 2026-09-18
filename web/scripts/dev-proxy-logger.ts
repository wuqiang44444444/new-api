// HPM passes printf arguments separately. Never log its request URL or Error
// message: both can contain tokens, signed query strings or upstream details.
export function createDevProxyLogger(target: string, route: string) {
  const destination = new URL(target).origin
  const prefix = `[dev-proxy] ${route} -> ${destination}`
  return {
    info() {},
    warn() {
      console.warn(`${prefix}: proxy warning`)
    },
    error(...args: unknown[]) {
      const error = args[3] ?? args[0]
      const candidate =
        typeof error === 'object' && error !== null && 'code' in error
          ? error.code
          : error
      const code =
        typeof candidate === 'string' &&
        /^[A-Z][A-Z0-9_]{0,63}$/.test(candidate)
          ? candidate
          : 'UNKNOWN_PROXY_ERROR'
      console.error(`${prefix}: ${code}`)
    },
  }
}
