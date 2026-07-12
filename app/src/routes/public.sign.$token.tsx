import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { PublicSigningPage } from '@/features/public-signing/components/PublicSigningPage'
import { resolveExplicitPublicLanguage } from '@/features/public-signing/public-signing-language'
import { resolveEmbeddedMode } from '@/features/public-signing/public-signing-embed'

const publicSignSearchSchema = z.object({
  language: z.string().optional(),
  embedded: z.union([z.string(), z.number(), z.boolean()]).optional(),
})

export const Route = createFileRoute('/public/sign/$token')({
  component: PublicSignRoute,
  validateSearch: publicSignSearchSchema,
})

function PublicSignRoute() {
  const { token } = Route.useParams()
  const search = Route.useSearch()
  return (
    <PublicSigningPage
      token={token}
      language={resolveExplicitPublicLanguage(search)}
      embedded={resolveEmbeddedMode(search)}
    />
  )
}
