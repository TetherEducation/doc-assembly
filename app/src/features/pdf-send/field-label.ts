import type { FieldType } from './model'

/** The label shown for a field type, on boxes and on the palette. */
export function fieldLabel(t: (key: string, fallback: string) => string, type: FieldType): string {
  switch (type) {
    case 'SIGNATURE':
      return t('pdfSend.field.signature', 'Signature')
    case 'DATE':
      return t('pdfSend.field.date', 'Date')
    case 'TEXT':
      return t('pdfSend.field.text', 'Text')
    case 'INITIALS':
      return t('pdfSend.field.initials', 'Initials')
  }
}
