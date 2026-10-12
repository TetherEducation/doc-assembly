import { StrictMode, type FormEvent } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { fetchMyRoles } from '@/features/auth/api/auth-api'
import {
  completeRedirectLogin,
  getUserInfo,
  loginWithCredentials,
  startRedirectLogin,
  type TokenResponse,
} from '@/lib/oidc'
import { useAuthStore } from '@/stores/auth-store'
import { useLogin } from './useLogin'

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === 'string' ? fallback : key),
  }),
}))

vi.mock('@/features/auth/api/auth-api', () => ({
  fetchMyRoles: vi.fn(),
}))

vi.mock('@/lib/oidc', () => ({
  completeRedirectLogin: vi.fn(),
  getUserInfo: vi.fn(),
  loginWithCredentials: vi.fn(),
  startRedirectLogin: vi.fn(),
}))

const TOKENS: TokenResponse = {
  access_token: 'access',
  refresh_token: 'refresh',
  expires_in: 300,
  refresh_expires_in: 1800,
  token_type: 'Bearer',
}

function visit(pathAndQuery: string) {
  window.history.replaceState(null, '', pathAndQuery)
}

beforeEach(() => {
  vi.restoreAllMocks()
  vi.clearAllMocks()
  useAuthStore.getState().clearAuth(false)
  vi.mocked(getUserInfo).mockResolvedValue({ sub: 'user-1', email: 'staff@example.com' })
  vi.mocked(fetchMyRoles).mockResolvedValue([])
  visit('/team/login')
})

describe('useLogin — redirect callback', () => {
  it('exchanges the code once under StrictMode, strips the callback query and finishes the login', async () => {
    visit('/team/login?code=the-code&state=s1&session_state=ss&iss=https%3A%2F%2Fidp&keep=1')
    vi.mocked(completeRedirectLogin).mockResolvedValue(TOKENS)

    const { result } = renderHook(() => useLogin(), { wrapper: StrictMode })

    expect(result.current.isLoading).toBe(true)
    await waitFor(() => expect(navigate).toHaveBeenCalledWith({ to: '/select-tenant' }))
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(completeRedirectLogin).toHaveBeenCalledTimes(1)
    expect(completeRedirectLogin).toHaveBeenCalledWith(
      '?code=the-code&state=s1&session_state=ss&iss=https%3A%2F%2Fidp&keep=1'
    )
    expect(window.location.pathname).toBe('/team/login')
    expect(window.location.search).toBe('?keep=1')
    expect(useAuthStore.getState().token).toBe('access')
    expect(useAuthStore.getState().userProfile?.id).toBe('user-1')
    expect(result.current.error).toBeNull()
  })

  it('strips an ignored callback and leaves the form idle', async () => {
    visit('/team/login?code=foreign&state=not-ours')
    vi.mocked(completeRedirectLogin).mockResolvedValue(null)

    const { result } = renderHook(() => useLogin())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(window.location.search).toBe('')
    expect(navigate).not.toHaveBeenCalled()
    expect(result.current.error).toBeNull()
    expect(useAuthStore.getState().token).toBeNull()
  })

  it('surfaces a provider error and strips it from the URL', async () => {
    visit('/team/login?error=access_denied&error_description=User+cancelled+login&state=s1')
    vi.mocked(completeRedirectLogin).mockRejectedValue(new Error('User cancelled login'))
    vi.spyOn(console, 'error').mockImplementation(() => {})

    const { result } = renderHook(() => useLogin())

    await waitFor(() => expect(result.current.error).toBe('User cancelled login'))
    expect(result.current.isLoading).toBe(false)
    expect(window.location.search).toBe('')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('does nothing on a plain visit to the login page', () => {
    const { result } = renderHook(() => useLogin())

    expect(result.current.isLoading).toBe(false)
    expect(completeRedirectLogin).not.toHaveBeenCalled()
  })
})

describe('useLogin — starting a redirect login', () => {
  it('passes the IdP hint through and stays redirecting', async () => {
    vi.mocked(startRedirectLogin).mockResolvedValue(undefined)
    const { result } = renderHook(() => useLogin())

    await act(() => result.current.startRedirectLogin({ idpHint: 'Google' }))

    expect(startRedirectLogin).toHaveBeenCalledWith({ idpHint: 'Google' })
    expect(result.current.isRedirecting).toBe(true)
    expect(result.current.error).toBeNull()
  })

  it('reports a failure to start and stops redirecting', async () => {
    vi.mocked(startRedirectLogin).mockRejectedValue(
      new Error('Redirect login is unavailable: the server did not provide an authorization endpoint.')
    )
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderHook(() => useLogin())

    await act(() => result.current.startRedirectLogin())

    expect(result.current.isRedirecting).toBe(false)
    expect(result.current.error).toMatch(/authorization endpoint/)
  })
})

describe('useLogin — password login', () => {
  it('goes through the same post-token steps', async () => {
    vi.mocked(loginWithCredentials).mockResolvedValue(TOKENS)
    const { result } = renderHook(() => useLogin())

    act(() => {
      result.current.setUsername('staff')
      result.current.setPassword('secret')
    })
    await act(() =>
      result.current.handleLogin({ preventDefault: vi.fn() } as unknown as FormEvent)
    )

    expect(loginWithCredentials).toHaveBeenCalledWith('staff', 'secret')
    expect(useAuthStore.getState().token).toBe('access')
    expect(useAuthStore.getState().userProfile?.id).toBe('user-1')
    expect(navigate).toHaveBeenCalledWith({ to: '/select-tenant' })
    expect(result.current.isLoading).toBe(false)
  })
})
