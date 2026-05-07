import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { PublicDocumentAccessPage } from '@/features/public-signing/components/PublicDocumentAccessPage'
import { resolveExplicitPublicLanguage } from '@/features/public-signing/public-signing-language'

const publicDocSearchSchema = z.object({
  language: z.string().optional(),
})

export const Route = createFileRoute('/public/doc/$documentId')({
  component: PublicDocAccessRoute,
  validateSearch: publicDocSearchSchema,
})

function PublicDocAccessRoute() {
  const { documentId } = Route.useParams()
  const search = Route.useSearch()
  return (
    <PublicDocumentAccessPage
      documentId={documentId}
      language={resolveExplicitPublicLanguage(search)}
    />
  )
}
