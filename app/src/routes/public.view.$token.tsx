import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { ReadonlyContractViewPage } from '@/features/readonly-view/components/ReadonlyContractViewPage'
import { resolveExplicitPublicLanguage } from '@/features/public-signing/public-signing-language'

const publicViewSearchSchema = z.object({
  language: z.string().optional(),
})

export const Route = createFileRoute('/public/view/$token')({
  component: PublicReadonlyViewRoute,
  validateSearch: publicViewSearchSchema,
})

function PublicReadonlyViewRoute() {
  const { token } = Route.useParams()
  const search = Route.useSearch()
  return (
    <ReadonlyContractViewPage
      token={token}
      language={resolveExplicitPublicLanguage(search)}
    />
  )
}
