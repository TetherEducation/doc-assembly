import { describe, expect, it } from 'vitest'
import {
  appendPublicLanguage,
  resolveExplicitPublicLanguage,
} from './public-signing-language'

describe('public signing language contract', () => {
  it('resolves supported explicit language values from URL search params', () => {
    expect(resolveExplicitPublicLanguage({ language: 'en' })).toBe('en')
    expect(resolveExplicitPublicLanguage({ language: 'es' })).toBe('es')
  })

  it('falls back to English for unsupported explicit language values', () => {
    expect(resolveExplicitPublicLanguage({ language: 'fr' })).toBe('en')
    expect(resolveExplicitPublicLanguage({ language: '' })).toBe('en')
  })

  it('leaves existing detection untouched when language is missing', () => {
    expect(resolveExplicitPublicLanguage({})).toBeUndefined()
  })

  it('appends the language query parameter to public flow URLs', () => {
    expect(appendPublicLanguage('/public/sign/token-1', 'es')).toBe(
      '/public/sign/token-1?language=es',
    )
    expect(appendPublicLanguage('/public/sign/token-1?foo=bar', 'en')).toBe(
      '/public/sign/token-1?foo=bar&language=en',
    )
  })

  it('does not append anything when no explicit language is present', () => {
    expect(appendPublicLanguage('/public/sign/token-1', undefined)).toBe(
      '/public/sign/token-1',
    )
  })
})
