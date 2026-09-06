import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

import { toAbsoluteAppUrl } from './app-url'

const ORIGIN = 'https://sign.tether.education'

function withBasePath(basePath: string, fn: () => void) {
  vi.stubEnv('VITE_BASE_PATH', basePath)
  try {
    fn()
  } finally {
    vi.unstubAllEnvs()
  }
}

beforeEach(() => {
  vi.stubGlobal('window', { location: { origin: ORIGIN } })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('toAbsoluteAppUrl', () => {
  it('does not prefix a path the API already prefixed', () => {
    // The read-only view link arrives as /team/public/view/{token}. Prefixing it
    // again yielded /team/team/... which renders "not found" with HTTP 200.
    withBasePath('/team', () => {
      expect(toAbsoluteAppUrl('/team/public/view/tok123')).toBe(
        `${ORIGIN}/team/public/view/tok123`
      )
    })
  })

  it('prefixes a path the API left unprefixed', () => {
    // The signed-PDF download link arrives as /public/sign/{token}/download.
    // Without the prefix the load balancer routes it to a different service.
    withBasePath('/team', () => {
      expect(toAbsoluteAppUrl('/public/sign/tok123/download')).toBe(
        `${ORIGIN}/team/public/sign/tok123/download`
      )
    })
  })

  it('leaves an absolute URL untouched', () => {
    withBasePath('/team', () => {
      expect(toAbsoluteAppUrl('https://elsewhere.example/x')).toBe(
        'https://elsewhere.example/x'
      )
    })
  })

  it('does not treat a merely-similar prefix as the base path', () => {
    withBasePath('/team', () => {
      expect(toAbsoluteAppUrl('/teamwork/x')).toBe(`${ORIGIN}/team/teamwork/x`)
    })
  })

  it('is a no-op when no base path is configured', () => {
    withBasePath('', () => {
      expect(toAbsoluteAppUrl('/public/view/tok123')).toBe(
        `${ORIGIN}/public/view/tok123`
      )
    })
  })

  it('normalises a path given without a leading slash', () => {
    withBasePath('/team', () => {
      expect(toAbsoluteAppUrl('public/view/tok123')).toBe(
        `${ORIGIN}/team/public/view/tok123`
      )
    })
  })

  it('tolerates a base path with a trailing slash', () => {
    withBasePath('/team/', () => {
      expect(toAbsoluteAppUrl('/team/public/view/tok123')).toBe(
        `${ORIGIN}/team/public/view/tok123`
      )
    })
  })
})
