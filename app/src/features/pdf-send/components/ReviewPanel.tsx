import { useTranslation } from 'react-i18next'

import { fieldLabel } from '../field-label'
import { FIELD_TYPES, countBoxes, type Box, type Problem, type Signer } from '../model'
import { signerColor } from '../signer-colors'
import { describeProblem, signerLabel } from '../describe-problem'

interface ReviewPanelProps {
  title: string
  signers: Signer[]
  boxes: Box[]
  numPages: number
  problems: Problem[]
}

/**
 * What will be sent: the title, the signers in order with their boxes by
 * type, and, while anything blocks Send, why.
 */
export function ReviewPanel({ title, signers, boxes, numPages, problems }: ReviewPanelProps) {
  const { t } = useTranslation()
  return (
    <section className="space-y-2" aria-labelledby="pdf-send-review">
      <h2 id="pdf-send-review" className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
        {t('pdfSend.review', 'Review')}
      </h2>
      <p className="text-sm">
        <span className="font-medium">{title.trim() || t('pdfSend.untitled', 'Untitled')}</span>
        <span className="text-muted-foreground"> · {t('pdfSend.pageCount', '{{n}} pages', { n: numPages })}</span>
      </p>
      <ol className="space-y-1 text-sm">
        {signers.map((signer, index) => {
          const counts = countBoxes(signer.id, boxes)
          const summary = FIELD_TYPES.filter((type) => counts[type] > 0)
            .map((type) => `${counts[type]} ${fieldLabel(t, type).toLowerCase()}`)
            .join(', ')
          return (
            <li key={signer.id} className="flex items-baseline gap-2">
              <span className="h-2.5 w-2.5 shrink-0" style={{ backgroundColor: signerColor(index) }} aria-hidden />
              <span>
                {index + 1}. {signerLabel(t, signers, index)}
                <span className="text-muted-foreground"> — {summary || t('pdfSend.noBoxes', 'no boxes yet')}</span>
              </span>
            </li>
          )
        })}
      </ol>
      {problems.length > 0 && (
        <ul className="space-y-1 text-xs text-destructive" data-testid="pdf-send-problems">
          {problems.map((problem, i) => (
            <li key={i}>• {describeProblem(t, problem, signers)}</li>
          ))}
        </ul>
      )}
    </section>
  )
}
