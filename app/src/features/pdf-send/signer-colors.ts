// One colour per signer, in signing order, distinct on a white page.
const SIGNER_COLORS = ['#2563eb', '#db2777', '#059669', '#d97706', '#7c3aed', '#0891b2', '#dc2626', '#4d7c0f']

export function signerColor(index: number): string {
  return SIGNER_COLORS[((index % SIGNER_COLORS.length) + SIGNER_COLORS.length) % SIGNER_COLORS.length]
}
