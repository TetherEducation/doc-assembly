import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { initOIDCConfig } from '@/lib/oidc'

import { completeLogin, type LoginStore } from './complete-login'

const API_BASE = '/team/api/v1'

const TOKENS = {
  access_token: fakeJwt({ email: 'ana@tether.education' }),
  refresh_token: 'refresh',
  expires_in: 300,
  refresh_expires_in: 1800,
  token_type: 'Bearer',
}

// An unsigned JWT is enough: the email is only shown, never trusted.
function fakeJwt(claims: Record<string, unknown>): string {
  const enc = (o: unknown) =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  return `${enc({ alg: 'none' })}.${enc(claims)}.sig`
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function recordingStore(): LoginStore & { calls: string[] } {
  const calls: string[] = []
  return {
    calls,
    setUserProfile: vi.fn(() => {
      calls.push('profile')
    }),
    setAllRoles: vi.fn(() => {
      calls.push('roles')
    }),
    setTokens: vi.fn(() => {
      calls.push('tokens')
    }),
  }
}

function routes(handlers: Record<string, () => Response>) {
  return vi.fn(async (url: string) => {
    for (const [prefix, handler] of Object.entries(handlers)) {
      if (url.startsWith(prefix)) return handler()
    }
    throw new Error(`unexpected request to ${url}`)
  })
}

beforeEach(() => {
  initOIDCConfig({
    tokenEndpoint: 'https://kc/token',
    userinfoEndpoint: 'https://kc/userinfo',
    endSessionEndpoint: 'https://kc/logout',
    clientId: 'panel',
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('completeLogin', () => {
  it('checks access and reads the profile with the new token, storing the tokens last', async () => {
    const store = recordingStore()
    const fetchMock = routes({
      [`${API_BASE}/me/roles`]: () => jsonResponse(200, { roles: [{ type: 'SYSTEM', role: 'SUPERADMIN' }] }),
      'https://kc/userinfo': () =>
        jsonResponse(200, { sub: 's', email: 'ana@tether.education', given_name: 'Ana' }),
    })

    const outcome = await completeLogin(TOKENS, {
      apiBase: API_BASE,
      store,
      fetchImpl: fetchMock as unknown as typeof fetch,
    })

    expect(outcome).toEqual({ kind: 'signed-in' })
    // Tokens last: storing them makes the root guard leave /login at once.
    expect(store.calls).toEqual(['profile', 'roles', 'tokens'])
    for (const call of fetchMock.mock.calls) {
      const init = (call as unknown as [string, RequestInit])[1]
      expect(init.headers).toEqual(expect.objectContaining({ Authorization: `Bearer ${TOKENS.access_token}` }))
    }
    expect(store.setAllRoles).toHaveBeenCalledWith([{ type: 'SYSTEM', role: 'SUPERADMIN' }])
    expect(store.setTokens).toHaveBeenCalledWith(TOKENS.access_token, 'refresh', 300)
  })

  it('reports not-invited for a valid login with no account, stores nothing and ends the session', async () => {
    const store = recordingStore()
    const fetchMock = routes({
      [`${API_BASE}/me/roles`]: () => jsonResponse(403, { error: 'user not found' }),
      'https://kc/logout': () => new Response(null, { status: 204 }),
    })

    const outcome = await completeLogin(TOKENS, {
      apiBase: API_BASE,
      store,
      fetchImpl: fetchMock as unknown as typeof fetch,
    })

    expect(outcome).toEqual({ kind: 'not-invited', email: 'ana@tether.education' })
    expect(store.calls).toEqual([])
    const logout = fetchMock.mock.calls.find((c) => (c as unknown as [string])[0] === 'https://kc/logout')
    expect(logout).toBeDefined()
    const body = new URLSearchParams(String((logout as unknown as [string, RequestInit])[1].body))
    expect(body.get('refresh_token')).toBe('refresh')
    expect(body.get('client_id')).toBe('panel')
  })

  it('reports rejected when the API refuses the token (401), stores nothing and ends the session', async () => {
    const store = recordingStore()
    const fetchMock = routes({
      [`${API_BASE}/me/roles`]: () => jsonResponse(401, {}),
      'https://kc/logout': () => new Response(null, { status: 204 }),
    })

    const outcome = await completeLogin(TOKENS, {
      apiBase: API_BASE,
      store,
      fetchImpl: fetchMock as unknown as typeof fetch,
    })

    expect(outcome).toEqual({ kind: 'rejected', status: 401 })
    expect(store.calls).toEqual([])
  })

  it('lets other role failures through with no roles, as before', async () => {
    for (const roles of [
      () => jsonResponse(500, {}),
      () => jsonResponse(403, { error: 'forbidden' }),
      () => {
        throw new TypeError('offline')
      },
    ]) {
      const store = recordingStore()
      const fetchMock = routes({
        [`${API_BASE}/me/roles`]: roles,
        'https://kc/userinfo': () => jsonResponse(200, { sub: 's' }),
      })
      const outcome = await completeLogin(TOKENS, {
        apiBase: API_BASE,
        store,
        fetchImpl: fetchMock as unknown as typeof fetch,
      })
      expect(outcome).toEqual({ kind: 'signed-in' })
      expect(store.setAllRoles).toHaveBeenCalledWith([])
      expect(store.calls.at(-1)).toBe('tokens')
    }
  })

  it('stores nothing when the profile cannot be read', async () => {
    const store = recordingStore()
    const fetchMock = routes({
      [`${API_BASE}/me/roles`]: () => jsonResponse(200, { roles: [] }),
      'https://kc/userinfo': () => jsonResponse(500, {}),
    })

    await expect(
      completeLogin(TOKENS, { apiBase: API_BASE, store, fetchImpl: fetchMock as unknown as typeof fetch })
    ).rejects.toThrow()
    expect(store.calls).toEqual([])
  })
})
