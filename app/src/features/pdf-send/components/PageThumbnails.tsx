import { useTranslation } from 'react-i18next'
import { Thumbnail } from 'react-pdf'

import type { Box } from '../model'

interface PageThumbnailsProps {
  numPages: number
  boxes: Box[]
  onJump: (pageNumber: number) => void
}

/** The page strip beside the pages: a thumbnail per page, with its box count. Must sit inside react-pdf's Document. */
export function PageThumbnails({ numPages, boxes, onJump }: PageThumbnailsProps) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t('pdfSend.pages', 'Pages')} className="space-y-3">
      {Array.from({ length: numPages }, (_, i) => i + 1).map((pageNumber) => {
        const count = boxes.filter((b) => b.page === pageNumber).length
        return (
          <div key={pageNumber} className="space-y-1 text-center">
            <Thumbnail
              pageNumber={pageNumber}
              width={88}
              className="mx-auto block border border-border hover:border-foreground"
              onItemClick={({ pageNumber: clicked }) => onJump(clicked)}
            />
            <div className="font-mono text-[10px] text-muted-foreground">
              {pageNumber}
              {count > 0 && ` · ${t('pdfSend.boxCount', '{{n}} boxes', { n: count })}`}
            </div>
          </div>
        )
      })}
    </nav>
  )
}
