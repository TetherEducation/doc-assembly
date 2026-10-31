import type { Problem, Signer } from './model'

type T = (key: string, fallback: string, options?: Record<string, unknown>) => string

/** A signer's name, or "Signer n" while it has none. */
export function signerLabel(t: T, signers: Signer[], index: number): string {
  return signers[index]?.name.trim() || t('pdfSend.unnamedSigner', 'Signer {{n}}', { n: index + 1 })
}

/** Why Send is blocked, naming the signer where there is one. */
export function describeProblem(t: T, problem: Problem, signers: Signer[]): string {
  switch (problem.kind) {
    case 'no-title':
      return t('pdfSend.problem.noTitle', 'Give the document a title.')
    case 'title-too-long':
      return t('pdfSend.problem.titleTooLong', 'Shorten the title to 200 characters or fewer.')
    case 'no-signers':
      return t('pdfSend.problem.noSigners', 'Add at least one signer.')
    case 'signer-name':
      return t('pdfSend.problem.signerName', 'Signer {{n}} needs a name.', { n: problem.signerIndex + 1 })
    case 'signer-email':
      return t('pdfSend.problem.signerEmail', '{{name}} needs a valid email.', { name: signerLabel(t, signers, problem.signerIndex) })
    case 'duplicate-name':
      return t('pdfSend.problem.duplicateName', 'Two signers are called {{name}}; give each a different name.', {
        name: signerLabel(t, signers, problem.signerIndex),
      })
    case 'no-signature':
      return t('pdfSend.problem.noSignature', '{{name}} has no signature box.', { name: signerLabel(t, signers, problem.signerIndex) })
    case 'box-outside-page':
      return t('pdfSend.problem.boxOutsidePage', 'A box is outside its page; move it back onto the page.')
  }
}
