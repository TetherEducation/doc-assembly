import { createHash } from 'node:crypto'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'

import { completeRedirectLogin, initOIDCConfig, startRedirectLogin } from './oidc'

const ORIGIN = 'https://sign.tether.education'
const AUTHORIZATION_ENDPOINT =
  'https://auth.example/realms/tether-team/protocol/openid-connect/auth'
const TOKEN_ENDPOINT = 'https://auth.example/realms/tether-team/protocol/openid-connect/token'
const CLIENT_ID = 'team-signing'
const STORAGE_KEY = 'doc-assembly:oidc-redirect-login'

const TOKENS = {
  access_token: 'access',
  refresh_token: 'refresh',
  expires_in: 300,
  refresh_expires_in: 1800,
  token_type: 'Bearer',
}

interface StoredAttempt {
  state: string
  verifier: string
  redirectUri: string
  createdAt: number
}

function createMemoryStorage(): Storage {
  const items = new Map<string, string>()
  return {
    get length() {
      return items.size
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => Array.from(items.keys())[index] ?? null,
    removeItem: (key) => {
      items.delete(key)
    },
    setItem: (key, value) => {
      items.set(key, String(value))
    },
  }
}

function initConfig(overrides: { authorizationEndpoint?: string } = {}) {
  initOIDCConfig({
    authorizationEndpoint: AUTHORIZATION_ENDPOINT,
    tokenEndpoint: TOKEN_ENDPOINT,
    userinfoEndpoint: '',
    endSessionEndpoint: '',
    clientId: CLIENT_ID,
    ...overrides,
  })
}

let storage: Storage
let assign: Mock
let fetchMock: Mock

beforeEach(() => {
  storage = createMemoryStorage()
  assign = vi.fn()
  fetchMock = vi.fn()
  vi.stubGlobal('window', { location: { origin: ORIGIN, assign }, sessionStorage: storage })
  vi.stubGlobal('fetch', fetchMock)
  vi.stubEnv('VITE_BASE_PATH', '/team')
  initConfig()
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

function storedAttempt(): StoredAttempt | null {
  const raw = storage.getItem(STORAGE_KEY)
  return raw ? (JSON.parse(raw) as StoredAttempt) : null
}

/** Starts a redirect login and returns the URL the browser was sent to. */
async function start(options?: Parameters<typeof startRedirectLogin>[0]): Promise<URL> {
  await startRedirectLogin(options)
  expect(assign).toHaveBeenCalledTimes(1)
  return new URL(assign.mock.calls[0][0] as string)
}

function respondWith(status: number, body: unknown) {
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  )
}

describe('startRedirectLogin', () => {
  it('sends the browser to the authorization endpoint with an S256 challenge for the stored verifier', async () => {
    const url = await start()
    const params = url.searchParams
    const stored = storedAttempt()

    expect(`${url.origin}${url.pathname}`).toBe(AUTHORIZATION_ENDPOINT)
    expect(params.get('response_type')).toBe('code')
    expect(params.get('client_id')).toBe(CLIENT_ID)
    expect(params.get('redirect_uri')).toBe(`${ORIGIN}/team/login`)
    expect(params.get('scope')).toBe('openid profile email')
    expect(params.get('code_challenge_method')).toBe('S256')
    expect(params.has('kc_idp_hint')).toBe(false)

    expect(stored).not.toBeNull()
    expect(params.get('state')).toBe(stored!.state)
    expect(stored!.redirectUri).toBe(`${ORIGIN}/team/login`)
    // 32 random bytes, base64url without padding.
    expect(stored!.verifier).toMatch(/^[A-Za-z0-9_-]{43}$/)
    expect(params.get('code_challenge')).toBe(
      createHash('sha256').update(stored!.verifier).digest('base64url')
    )
  })

  it('adds kc_idp_hint only when an IdP hint is given', async () => {
    const url = await start({ idpHint: 'Google' })

    expect(url.searchParams.get('kc_idp_hint')).toBe('Google')
  })

  it('uses an explicit redirect URI for both the request and the stored attempt', async () => {
    const url = await start({ redirectUri: 'http://localhost:5173/login' })

    expect(url.searchParams.get('redirect_uri')).toBe('http://localhost:5173/login')
    expect(storedAttempt()!.redirectUri).toBe('http://localhost:5173/login')
  })

  it('refuses to start without an authorization endpoint instead of guessing one', async () => {
    initConfig({ authorizationEndpoint: undefined })

    await expect(startRedirectLogin()).rejects.toThrow(/authorization endpoint/)
    expect(assign).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })
})

describe('completeRedirectLogin', () => {
  it('returns null for a URL that is not a callback, leaving a started attempt alone', async () => {
    await start()

    await expect(completeRedirectLogin('?foo=bar')).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).not.toBeNull()
  })

  it('ignores a code whose state does not match, without calling the token endpoint', async () => {
    await start()

    await expect(
      completeRedirectLogin('?code=foreign-code&state=not-ours')
    ).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })

  it('ignores a code that comes back without a state', async () => {
    await start()

    await expect(completeRedirectLogin('?code=foreign-code')).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })

  it('ignores a code when this tab never started a redirect login', async () => {
    // e.g. Keycloak bouncing back from a password-reset link
    await expect(
      completeRedirectLogin('?code=foreign-code&state=whatever&session_state=s')
    ).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('ignores a code once the attempt is older than ten minutes', async () => {
    const state = (await start()).searchParams.get('state')
    const { createdAt } = storedAttempt()!
    vi.spyOn(Date, 'now').mockReturnValue(createdAt + 10 * 60 * 1000 + 1)

    await expect(
      completeRedirectLogin(`?code=late-code&state=${state}`)
    ).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })

  it('throws the provider error and clears the stored attempt', async () => {
    const state = (await start()).searchParams.get('state')

    await expect(
      completeRedirectLogin(
        `?error=access_denied&error_description=User+cancelled+login&state=${state}`
      )
    ).rejects.toThrow('User cancelled login')
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })

  it('falls back to the error code when the provider gives no description', async () => {
    const state = (await start()).searchParams.get('state')

    await expect(
      completeRedirectLogin(`?error=access_denied&state=${state}`)
    ).rejects.toThrow('access_denied')
  })

  it('ignores an error whose state does not match, so a crafted link cannot set the message', async () => {
    await start()

    await expect(
      completeRedirectLogin('?error=x&error_description=Call+this+number&state=not-ours')
    ).resolves.toBeNull()
    await expect(
      completeRedirectLogin('?error=x&error_description=Call+this+number')
    ).resolves.toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(storedAttempt()).toBeNull()
  })

  it('exchanges a matching code with the stored verifier and redirect URI', async () => {
    const state = (await start()).searchParams.get('state')
    const stored = storedAttempt()!
    respondWith(200, TOKENS)

    const tokens = await completeRedirectLogin(
      `?code=the-code&state=${state}&session_state=abc&iss=https%3A%2F%2Fauth.example`
    )

    expect(tokens).toEqual(TOKENS)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [endpoint, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(endpoint).toBe(TOKEN_ENDPOINT)
    expect(init.method).toBe('POST')
    expect(init.headers).toEqual({ 'Content-Type': 'application/x-www-form-urlencoded' })

    const body = new URLSearchParams(init.body as string)
    expect(Object.fromEntries(body)).toEqual({
      grant_type: 'authorization_code',
      code: 'the-code',
      redirect_uri: stored.redirectUri,
      client_id: CLIENT_ID,
      code_verifier: stored.verifier,
    })
    expect(storedAttempt()).toBeNull()
  })

  it('redeems a state only once', async () => {
    const state = (await start()).searchParams.get('state')
    respondWith(200, TOKENS)
    const query = `?code=the-code&state=${state}`

    await expect(completeRedirectLogin(query)).resolves.toEqual(TOKENS)
    await expect(completeRedirectLogin(query)).resolves.toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('redeems a state only once even when called twice at the same time', async () => {
    const state = (await start()).searchParams.get('state')
    respondWith(200, TOKENS)
    const query = `?code=the-code&state=${state}`

    const results = await Promise.all([
      completeRedirectLogin(query),
      completeRedirectLogin(query),
    ])

    expect(results.filter(Boolean)).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('reports a failed exchange and still spends the attempt', async () => {
    const state = (await start()).searchParams.get('state')
    respondWith(400, { error: 'invalid_grant', error_description: 'Code not valid' })

    await expect(
      completeRedirectLogin(`?code=the-code&state=${state}`)
    ).rejects.toThrow('Code not valid')
    expect(storedAttempt()).toBeNull()
  })
})
