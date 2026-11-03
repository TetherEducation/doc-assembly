import { isAxiosError } from 'axios'
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
    case 'name-too-long':
      return t('pdfSend.problem.nameTooLong', 'Shorten the name of signer {{n}}.', { n: problem.signerIndex + 1 })
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

/**
 * What to tell the sender when Send fails. A 4xx means the server refused the
 * request before creating anything, so its reason is shown and retrying is safe.
 * Anything else (timeout, network error, 5xx) may have failed after the document
 * was created and the signers emailed, so a retry could send it twice.
 */
export function describeSendError(t: T, error: unknown): string {
  if (isAxiosError(error)) {
    const status = error.response?.status
    const reason = error.response?.data?.error
    if (status !== undefined && status >= 400 && status < 500 && typeof reason === 'string') {
      return reason
    }
  }
  return t(
    'pdfSend.sendMaybeSent',
    'The send may have gone through. Check the Signing list before trying again, so it is not sent twice.'
  )
}
