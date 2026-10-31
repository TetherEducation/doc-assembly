/**
 * The "Send a PDF" editor's model and the pure rules around it
 * (tools-team-signing docs/SPEC-2.4-field-editor.md).
 *
 * Boxes are kept in PDF points with the origin at the TOP-left of the page as
 * pdf.js draws it (CropBox and /Rotate applied), so they don't depend on the
 * zoom. The engine wants points from the BOTTOM-left of that same frame
 * (POST /documents/from-pdf); toSendPdfPayload converts.
 */

export const FIELD_TYPES = ['SIGNATURE', 'INITIALS', 'DATE', 'TEXT'] as const
export type FieldType = (typeof FIELD_TYPES)[number]

export interface Signer {
  id: string
  name: string
  email: string
}

export interface Box {
  id: string
  signerId: string
  type: FieldType
  /** 1-based page number. */
  page: number
  /** Left edge, in points from the page's left. */
  x: number
  /** Top edge, in points from the page's top. */
  top: number
  width: number
  height: number
}

/** A page's size in points, as pdf.js draws it: getViewport({ scale: 1 }). */
export interface PageSize {
  width: number
  height: number
}

/** The engine's limits (POST /documents/from-pdf). */
export const MAX_PDF_BYTES = 12 * 1024 * 1024
export const MAX_TITLE_LENGTH = 200
export const MAX_PAGES = 2000
/**
 * A signer's name becomes the signer role's name (at most 100 bytes on the
 * server) and its signature anchor (`__sig_<name>__`, plus a `role_` prefix and
 * an order suffix when needed, also at most 100 bytes). 80 UTF-8 bytes keeps
 * both inside the limit; an accented letter takes 2.
 */
export const MAX_SIGNER_NAME_BYTES = 80
/**
 * How far a box may poke out of its page, and how far a page's visible area may
 * sit from (0,0), before the engine refuses it (pdfpages uses the same 0.5 pt).
 */
export const PAGE_TOLERANCE = 0.5

/** Default box sizes in points (SPEC-2.4 §3). */
export const DEFAULT_BOX_SIZE: Record<FieldType, { width: number; height: number }> = {
  SIGNATURE: { width: 180, height: 40 },
  INITIALS: { width: 60, height: 30 },
  DATE: { width: 100, height: 20 },
  TEXT: { width: 160, height: 20 },
}

/** The smallest box, in points; also what a drag must exceed to draw one. */
export const MIN_BOX_SIZE = 12

type Rect = Pick<Box, 'x' | 'top' | 'width' | 'height'>

/** Keeps a box inside its page, shrinking it only if it is bigger than the page. */
export function clampBox<T extends Rect>(box: T, page: PageSize): T {
  const width = Math.max(MIN_BOX_SIZE, Math.min(box.width, page.width))
  const height = Math.max(MIN_BOX_SIZE, Math.min(box.height, page.height))
  return {
    ...box,
    width,
    height,
    x: Math.min(Math.max(0, box.x), page.width - width),
    top: Math.min(Math.max(0, box.top), page.height - height),
  }
}

/** A point of the page, in points from its top-left, from a pointer position. */
export function pointOnPage(
  pointer: { clientX: number; clientY: number },
  pageRect: { left: number; top: number },
  scale: number
): { x: number; top: number } {
  return { x: (pointer.clientX - pageRect.left) / scale, top: (pointer.clientY - pageRect.top) / scale }
}

/**
 * The box a click or a drag on the page makes. A drag smaller than
 * MIN_BOX_SIZE counts as a click: a box of the type's default size, centred on
 * the point. Either way the box stays inside the page.
 */
export function boxFromDrag(
  base: Pick<Box, 'id' | 'signerId' | 'type' | 'page'>,
  pageSize: PageSize,
  start: { x: number; top: number },
  end: { x: number; top: number } = start
): Box {
  const width = Math.abs(end.x - start.x)
  const height = Math.abs(end.top - start.top)
  if (width < MIN_BOX_SIZE && height < MIN_BOX_SIZE) {
    const size = DEFAULT_BOX_SIZE[base.type]
    return clampBox({ ...base, x: start.x - size.width / 2, top: start.top - size.height / 2, ...size }, pageSize)
  }
  return clampBox(
    {
      ...base,
      x: Math.min(start.x, end.x),
      top: Math.min(start.top, end.top),
      width: Math.max(width, MIN_BOX_SIZE),
      height: Math.max(height, MIN_BOX_SIZE),
    },
    pageSize
  )
}

export type Corner = 'nw' | 'ne' | 'sw' | 'se'

/**
 * Resizes a box by dragging one corner by (dx, dy) points. The opposite corner
 * stays put, the box never gets smaller than MIN_BOX_SIZE, and it stays inside
 * the page.
 */
export function resizeFromCorner<T extends Rect>(box: T, corner: Corner, dx: number, dy: number, page: PageSize): T {
  let left = box.x
  let right = box.x + box.width
  let top = box.top
  let bottom = box.top + box.height
  if (corner === 'nw' || corner === 'sw') left = Math.min(Math.max(0, left + dx), right - MIN_BOX_SIZE)
  else right = Math.max(Math.min(page.width, right + dx), left + MIN_BOX_SIZE)
  if (corner === 'nw' || corner === 'ne') top = Math.min(Math.max(0, top + dy), bottom - MIN_BOX_SIZE)
  else bottom = Math.max(Math.min(page.height, bottom + dy), top + MIN_BOX_SIZE)
  return { ...box, x: left, top, width: right - left, height: bottom - top }
}

/** Moves a box by (dx, dy) points, keeping it inside the page. */
export function moveBox<T extends Rect>(box: T, dx: number, dy: number, page: PageSize): T {
  return clampBox({ ...box, x: box.x + dx, top: box.top + dy }, page)
}

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export type Problem =
  | { kind: 'no-title' }
  | { kind: 'title-too-long' }
  | { kind: 'no-signers' }
  | { kind: 'signer-name'; signerIndex: number }
  | { kind: 'signer-email'; signerIndex: number }
  | { kind: 'duplicate-name'; signerIndex: number }
  | { kind: 'name-too-long'; signerIndex: number }
  | { kind: 'no-signature'; signerIndex: number }
  | { kind: 'box-outside-page'; boxId: string }

/**
 * Everything that stops the document from being sent, in the order a person
 * would fix it. These are the rules the engine enforces (SPEC-2.4 §"Rules the
 * server enforces"); checking them here means the sender sees what is missing
 * instead of a bare 400. Signer order is the row number, so it is always 1..n
 * and unique.
 */
export function findProblems(title: string, signers: Signer[], boxes: Box[], pageSizes: PageSize[]): Problem[] {
  const problems: Problem[] = []
  if (!title.trim()) problems.push({ kind: 'no-title' })
  else if ([...title.trim()].length > MAX_TITLE_LENGTH) problems.push({ kind: 'title-too-long' })
  if (signers.length === 0) problems.push({ kind: 'no-signers' })
  const seen = new Set<string>()
  signers.forEach((signer, signerIndex) => {
    const name = signer.name.trim()
    if (!name) problems.push({ kind: 'signer-name', signerIndex })
    else if (new TextEncoder().encode(name).length > MAX_SIGNER_NAME_BYTES) problems.push({ kind: 'name-too-long', signerIndex })
    else if (seen.has(name.toLowerCase())) problems.push({ kind: 'duplicate-name', signerIndex })
    if (name) seen.add(name.toLowerCase())
    if (!EMAIL.test(signer.email.trim())) problems.push({ kind: 'signer-email', signerIndex })
    if (!boxes.some((b) => b.signerId === signer.id && b.type === 'SIGNATURE')) {
      problems.push({ kind: 'no-signature', signerIndex })
    }
  })
  for (const box of boxes) {
    const page = pageSizes[box.page - 1]
    const inside =
      !!page &&
      box.x >= -PAGE_TOLERANCE &&
      box.top >= -PAGE_TOLERANCE &&
      box.x + box.width <= page.width + PAGE_TOLERANCE &&
      box.top + box.height <= page.height + PAGE_TOLERANCE
    if (!inside) problems.push({ kind: 'box-outside-page', boxId: box.id })
  }
  return problems
}

/** How many boxes of each type a signer has, for the review. */
export function countBoxes(signerId: string, boxes: Box[]): Record<FieldType, number> {
  const counts: Record<FieldType, number> = { SIGNATURE: 0, INITIALS: 0, DATE: 0, TEXT: 0 }
  for (const box of boxes) if (box.signerId === signerId) counts[box.type]++
  return counts
}

export interface SendPdfPayload {
  title: string
  signers: {
    name: string
    email: string
    order: number
    fields: { type: FieldType; page: number; x: number; y: number; width: number; height: number }[]
  }[]
}

const round = (n: number) => Math.round(n * 100) / 100

/**
 * The engine's payload: signers in the order shown (order is the row number,
 * 1..n), each with their boxes, in points from the BOTTOM-left of the page:
 * y = H − (top + height).
 */
export function toSendPdfPayload(title: string, signers: Signer[], boxes: Box[], pageSizes: PageSize[]): SendPdfPayload {
  return {
    title: title.trim(),
    signers: signers.map((signer, index) => ({
      name: signer.name.trim(),
      email: signer.email.trim(),
      order: index + 1,
      fields: boxes
        .filter((box) => box.signerId === signer.id)
        .map((box) => ({
          type: box.type,
          page: box.page,
          x: round(box.x),
          y: round(pageSizes[box.page - 1].height - box.top - box.height),
          width: round(box.width),
          height: round(box.height),
        })),
    })),
  }
}

/** The title a PDF's file name suggests: no extension, separators as spaces. */
export function titleFromFileName(fileName: string): string {
  return fileName
    .replace(/\.pdf$/i, '')
    .replace(/[_-]+/g, ' ')
    .trim()
}
