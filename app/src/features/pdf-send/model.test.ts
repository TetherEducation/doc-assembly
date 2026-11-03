import { describe, expect, it } from 'vitest'

import {
  MAX_SIGNER_NAME_BYTES,
  MAX_TITLE_LENGTH,
  boxFromDrag,
  clampBox,
  countBoxes,
  findProblems,
  moveBox,
  pointOnPage,
  resizeFromCorner,
  titleFromFileName,
  toSendPdfPayload,
  type Box,
  type PageSize,
  type Signer,
} from './model'

// Page frames as pdf.js reports them: getViewport({ scale: 1 }).
const LETTER: PageSize = { width: 612, height: 792 }
const A4: PageSize = { width: 595.28, height: 841.89 }
const LEGAL: PageSize = { width: 612, height: 1008 }
const LANDSCAPE: PageSize = { width: 792, height: 612 }
// A portrait Letter page with /Rotate 90: pdf.js swaps the viewport, and the
// engine (doc-assembly#39) sizes it the same way, so it is placed like landscape.
const ROTATED_90: PageSize = { width: 792, height: 612 }

const ana: Signer = { id: 's1', name: 'Ana', email: 'ana@example.test' }
const ben: Signer = { id: 's2', name: 'Ben', email: 'ben@example.test' }
const box = (extra: Partial<Box> = {}): Box => ({
  id: 'b1',
  signerId: 's1',
  type: 'SIGNATURE',
  page: 1,
  x: 72,
  top: 100,
  width: 180,
  height: 40,
  ...extra,
})

describe('coordinate conversion (SPEC-2.4: x = left/s, y = H − (top + h)/s)', () => {
  it.each([
    ['Letter', LETTER],
    ['A4', A4],
    ['Legal', LEGAL],
    ['landscape', LANDSCAPE],
    ['rotated 90°', ROTATED_90],
  ])('flips y to the bottom-left origin on a %s page', (_name, page) => {
    const payload = toSendPdfPayload('T', [ana], [box()], [page])
    const field = payload.signers[0].fields[0]
    expect(field.x).toBe(72)
    expect(field.y).toBeCloseTo(page.height - 100 - 40, 2)
    expect(field.width).toBe(180)
    expect(field.height).toBe(40)
  })

  it.each([0.5, 1, 1.37, 2])('gives the same points at zoom %s', (scale) => {
    // A pointer at the CSS position of the point (100, 200) at this zoom.
    const point = pointOnPage({ clientX: 10 + 100 * scale, clientY: 20 + 200 * scale }, { left: 10, top: 20 }, scale)
    expect(point.x).toBeCloseTo(100, 6)
    expect(point.top).toBeCloseTo(200, 6)
  })

  it('uses each box\'s own page height on mixed page sizes', () => {
    const payload = toSendPdfPayload('T', [ana], [box({ page: 1 }), box({ id: 'b2', page: 2 })], [LETTER, A4])
    expect(payload.signers[0].fields.map((f) => f.y)).toEqual([652, 701.89])
  })
})

describe('toSendPdfPayload', () => {
  it('numbers signers by row, keeps each one\'s boxes, trims names and emails', () => {
    const payload = toSendPdfPayload(
      '  NDA  ',
      [{ ...ana, name: ' Ana ' }, { ...ben, email: ' ben@example.test ' }],
      [box(), box({ id: 'b2', signerId: 's2' }), box({ id: 'b3', signerId: 's2', type: 'DATE' })],
      [LETTER]
    )
    expect(payload.title).toBe('NDA')
    expect(payload.signers.map((s) => [s.name, s.email, s.order, s.fields.map((f) => f.type)])).toEqual([
      ['Ana', 'ana@example.test', 1, ['SIGNATURE']],
      ['Ben', 'ben@example.test', 2, ['SIGNATURE', 'DATE']],
    ])
  })
})

describe('findProblems: one test per server rule', () => {
  const ok = () => findProblems('NDA', [ana, ben], [box(), box({ id: 'b2', signerId: 's2' })], [LETTER])

  it('accepts a complete document', () => {
    expect(ok()).toEqual([])
  })

  it('needs a title', () => {
    expect(findProblems('  ', [ana], [box()], [LETTER])).toEqual([{ kind: 'no-title' }])
  })

  it(`takes a title of at most ${MAX_TITLE_LENGTH} characters`, () => {
    expect(findProblems('x'.repeat(MAX_TITLE_LENGTH), [ana], [box()], [LETTER])).toEqual([])
    expect(findProblems('x'.repeat(MAX_TITLE_LENGTH + 1), [ana], [box()], [LETTER])).toEqual([{ kind: 'title-too-long' }])
  })

  it(`takes a signer name of at most ${MAX_SIGNER_NAME_BYTES} UTF-8 bytes`, () => {
    const named = (name: string) => findProblems('NDA', [{ ...ana, name }], [box()], [LETTER])
    expect(named('x'.repeat(MAX_SIGNER_NAME_BYTES))).toEqual([])
    expect(named('x'.repeat(MAX_SIGNER_NAME_BYTES + 1))).toEqual([{ kind: 'name-too-long', signerIndex: 0 }])
    // An accented letter takes 2 bytes, as on the server.
    expect(named('á'.repeat(MAX_SIGNER_NAME_BYTES / 2))).toEqual([])
    expect(named('á'.repeat(MAX_SIGNER_NAME_BYTES / 2 + 1))).toEqual([{ kind: 'name-too-long', signerIndex: 0 }])
  })

  it('needs at least one signer', () => {
    expect(findProblems('NDA', [], [], [LETTER])).toEqual([{ kind: 'no-signers' }])
  })

  it('needs a name for every signer', () => {
    expect(findProblems('NDA', [{ ...ana, name: ' ' }], [box()], [LETTER])).toEqual([{ kind: 'signer-name', signerIndex: 0 }])
  })

  it('needs a valid email for every signer', () => {
    expect(findProblems('NDA', [{ ...ana, email: 'ana@' }], [box()], [LETTER])).toEqual([{ kind: 'signer-email', signerIndex: 0 }])
  })

  it('refuses two signers with the same name, ignoring case and spaces', () => {
    const problems = findProblems('NDA', [ana, { ...ben, name: ' ANA ' }], [box(), box({ id: 'b2', signerId: 's2' })], [LETTER])
    expect(problems).toEqual([{ kind: 'duplicate-name', signerIndex: 1 }])
  })

  it('needs a signature box for every signer, even one with other boxes', () => {
    const problems = findProblems('NDA', [ana, ben], [box(), box({ id: 'b2', signerId: 's2', type: 'DATE' })], [LETTER])
    expect(problems).toEqual([{ kind: 'no-signature', signerIndex: 1 }])
  })

  it('needs every box on a page of the PDF', () => {
    expect(findProblems('NDA', [ana], [box({ page: 2 })], [LETTER])).toContainEqual({ kind: 'box-outside-page', boxId: 'b1' })
  })

  it('allows a box to poke out of its page by at most 0.5 pt', () => {
    expect(findProblems('NDA', [ana], [box({ x: 612 - 180 + 0.5 })], [LETTER])).toEqual([])
    expect(findProblems('NDA', [ana], [box({ x: 612 - 180 + 0.6 })], [LETTER])).toEqual([{ kind: 'box-outside-page', boxId: 'b1' }])
  })
})

describe('placing and editing boxes', () => {
  const base = { id: 'b', signerId: 's1', page: 1 }

  it('a click drops a default-size box centred on the point (SPEC-2.4 sizes)', () => {
    expect(boxFromDrag({ ...base, type: 'SIGNATURE' }, LETTER, { x: 300, top: 400 })).toMatchObject({
      x: 210, top: 380, width: 180, height: 40,
    })
    expect(boxFromDrag({ ...base, type: 'INITIALS' }, LETTER, { x: 300, top: 400 })).toMatchObject({ width: 60, height: 30 })
    expect(boxFromDrag({ ...base, type: 'DATE' }, LETTER, { x: 300, top: 400 })).toMatchObject({ width: 100, height: 20 })
    expect(boxFromDrag({ ...base, type: 'TEXT' }, LETTER, { x: 300, top: 400 })).toMatchObject({ width: 160, height: 20 })
  })

  it('a click near the edge keeps the box inside the page', () => {
    expect(boxFromDrag({ ...base, type: 'SIGNATURE' }, LETTER, { x: 5, top: 790 })).toMatchObject({ x: 0, top: 752 })
  })

  it('a drag draws the box between the two points, in either direction', () => {
    expect(boxFromDrag({ ...base, type: 'TEXT' }, LETTER, { x: 400, top: 300 }, { x: 100, top: 250 })).toMatchObject({
      x: 100, top: 250, width: 300, height: 50,
    })
  })

  it('resizes from each corner, keeping the opposite corner still', () => {
    const b = box({ x: 100, top: 100, width: 100, height: 50 })
    expect(resizeFromCorner(b, 'se', 20, 10, LETTER)).toMatchObject({ x: 100, top: 100, width: 120, height: 60 })
    expect(resizeFromCorner(b, 'nw', -20, -10, LETTER)).toMatchObject({ x: 80, top: 90, width: 120, height: 60 })
    expect(resizeFromCorner(b, 'ne', 20, -10, LETTER)).toMatchObject({ x: 100, top: 90, width: 120, height: 60 })
    expect(resizeFromCorner(b, 'sw', -20, 10, LETTER)).toMatchObject({ x: 80, top: 100, width: 120, height: 60 })
  })

  it('never resizes below the minimum or past the page', () => {
    const b = box({ x: 100, top: 100, width: 100, height: 50 })
    expect(resizeFromCorner(b, 'se', -500, -500, LETTER)).toMatchObject({ width: 12, height: 12 })
    expect(resizeFromCorner(b, 'se', 5000, 5000, LETTER)).toMatchObject({ width: 512, height: 692 })
    expect(resizeFromCorner(b, 'nw', -5000, -5000, LETTER)).toMatchObject({ x: 0, top: 0, width: 200, height: 150 })
  })

  it('moves a box but not off the page', () => {
    expect(moveBox(box(), 10, 20, LETTER)).toMatchObject({ x: 82, top: 120 })
    expect(moveBox(box(), 9999, -9999, LETTER)).toMatchObject({ x: 432, top: 0 })
  })

  it('clampBox shrinks a box bigger than the page', () => {
    expect(clampBox({ x: 0, top: 0, width: 900, height: 20 }, LETTER).width).toBe(612)
  })
})

describe('review helpers', () => {
  it('counts each signer\'s boxes by type', () => {
    const counts = countBoxes('s1', [box(), box({ id: 'b2', type: 'DATE' }), box({ id: 'b3', type: 'DATE' }), box({ id: 'b4', signerId: 's2' })])
    expect(counts).toEqual({ SIGNATURE: 1, INITIALS: 0, DATE: 2, TEXT: 0 })
  })

  it('titleFromFileName drops the extension and turns separators into spaces', () => {
    expect(titleFromFileName('Mutual_NDA-2026.PDF')).toBe('Mutual NDA 2026')
  })
})
