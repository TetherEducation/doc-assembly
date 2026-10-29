import { useQuery } from '@tanstack/react-query'

import { getAuthConfig, type AuthConfig } from './auth-config'

export type ClientFeatures = NonNullable<AuthConfig['features']>

/**
 * The deployment's feature switches from GET /api/v1/config, which
 * getAuthConfig fetches once and caches. Undefined until loaded, so each caller
 * picks the default that is safe for its feature.
 */
export function useClientFeatures(): ClientFeatures | undefined {
  const { data } = useQuery({
    queryKey: ['client-config'],
    queryFn: getAuthConfig,
    staleTime: Infinity,
  })
  return data?.features
}
