import type { SupportedLanguage } from '@/lib/i18n'

type SearchValue = string | undefined

const publicSigningLanguages = ['en', 'es'] as const satisfies readonly SupportedLanguage[]

export type PublicSigningLanguage = SupportedLanguage

export function resolveExplicitPublicLanguage(search: {
  language?: SearchValue
}): PublicSigningLanguage | undefined {
  if (!Object.hasOwn(search, 'language')) {
    return undefined
  }

  const language = search.language
  if (publicSigningLanguages.includes(language as SupportedLanguage)) {
    return language as SupportedLanguage
  }

  return 'en'
}

export function appendPublicLanguage(
  path: string,
  language: PublicSigningLanguage | undefined,
): string {
  if (!language) return path

  const separator = path.includes('?') ? '&' : '?'
  return `${path}${separator}language=${encodeURIComponent(language)}`
}

export function publicLanguageOptions(
  language: PublicSigningLanguage | undefined,
): { language: PublicSigningLanguage } | undefined {
  return language ? { language } : undefined
}
