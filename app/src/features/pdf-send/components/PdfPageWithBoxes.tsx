import { useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Trash2 } from 'lucide-react'
import { Page } from 'react-pdf'

import { cn } from '@/lib/utils'

import { fieldLabel } from '../field-label'
import {
  FIELD_TYPES,
  PAGE_TOLERANCE,
  boxFromDrag,
  moveBox,
  pointOnPage,
  resizeFromCorner,
  type Box,
  type Corner,
  type FieldType,
  type PageSize,
  type Signer,
} from '../model'
import { signerColor } from '../signer-colors'

export interface PageInfo {
  size: PageSize
  /** The page's visible area doesn't start at (0, 0); the engine refuses it. */
  offset: boolean
}

interface PdfPageWithBoxesProps {
  pageNumber: number
  /** Width the page is drawn at, in CSS pixels. */
  renderWidth: number
  /** The page's frame in points, once react-pdf has loaded it. */
  pageSize: PageSize | undefined
  boxes: Box[]
  signers: Signer[]
  selectedBoxId: string | null
  /** The field type the next click or drag places, for the current signer. */
  activeTool: FieldType | null
  currentSignerId: string
  newBoxId: () => string
  onPageLoad: (pageNumber: number, info: PageInfo) => void
  onPlace: (box: Box) => void
  onSelect: (boxId: string | null) => void
  onChange: (box: Box) => void
  onDelete: (boxId: string) => void
}

type Drag =
  | { kind: 'move'; boxId: string; startX: number; startY: number; original: Box }
  | { kind: 'resize'; corner: Corner; boxId: string; startX: number; startY: number; original: Box }

const CORNERS: { corner: Corner; className: string; cursor: string }[] = [
  { corner: 'nw', className: '-left-1 -top-1', cursor: 'nwse-resize' },
  { corner: 'ne', className: '-right-1 -top-1', cursor: 'nesw-resize' },
  { corner: 'sw', className: '-bottom-1 -left-1', cursor: 'nesw-resize' },
  { corner: 'se', className: '-bottom-1 -right-1', cursor: 'nwse-resize' },
]

/**
 * One PDF page with the boxes placed on it. Boxes are stored in points and
 * drawn at the page's current scale. With a field type chosen, a click drops a
 * default-size box and a drag draws one. Dragging a box moves it; dragging a
 * corner resizes it. Everything stays inside the page.
 */
export function PdfPageWithBoxes({
  pageNumber,
  renderWidth,
  pageSize,
  boxes,
  signers,
  selectedBoxId,
  activeTool,
  currentSignerId,
  newBoxId,
  onPageLoad,
  onPlace,
  onSelect,
  onChange,
  onDelete,
}: PdfPageWithBoxesProps) {
  const { t } = useTranslation()
  const drag = useRef<Drag | null>(null)
  const [draw, setDraw] = useState<{ start: { x: number; top: number }; end: { x: number; top: number } } | null>(null)
  const scale = pageSize ? renderWidth / pageSize.width : 1

  const signerName = (signerId: string) => {
    const index = signers.findIndex((s) => s.id === signerId)
    return signers[index]?.name.trim() || t('pdfSend.unnamedSigner', 'Signer {{n}}', { n: index + 1 })
  }

  // --- drawing a new box on the page ---
  const onOverlayPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget || !pageSize) return
    if (!activeTool) {
      onSelect(null)
      return
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const point = pointOnPage(event, event.currentTarget.getBoundingClientRect(), scale)
    setDraw({ start: point, end: point })
  }
  const onOverlayPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!draw) return
    setDraw({ ...draw, end: pointOnPage(event, event.currentTarget.getBoundingClientRect(), scale) })
  }
  const onOverlayPointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!draw || !pageSize || !activeTool) return
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    const end = pointOnPage(event, event.currentTarget.getBoundingClientRect(), scale)
    onPlace(boxFromDrag({ id: newBoxId(), signerId: currentSignerId, type: activeTool, page: pageNumber }, pageSize, draw.start, end))
    setDraw(null)
  }

  // --- moving and resizing a placed box ---
  const startDrag = (event: ReactPointerEvent<HTMLElement>, next: Drag) => {
    event.stopPropagation()
    event.currentTarget.setPointerCapture?.(event.pointerId)
    drag.current = next
    onSelect(next.boxId)
  }
  const moveDrag = (event: ReactPointerEvent<HTMLElement>) => {
    const current = drag.current
    if (!current || !pageSize) return
    const dx = (event.clientX - current.startX) / scale
    const dy = (event.clientY - current.startY) / scale
    onChange(
      current.kind === 'move'
        ? moveBox(current.original, dx, dy, pageSize)
        : resizeFromCorner(current.original, current.corner, dx, dy, pageSize)
    )
  }
  const endDrag = (event: ReactPointerEvent<HTMLElement>) => {
    event.currentTarget.releasePointerCapture?.(event.pointerId)
    drag.current = null
  }

  const draft = draw && pageSize && activeTool
    ? boxFromDrag({ id: 'draft', signerId: currentSignerId, type: activeTool, page: pageNumber }, pageSize, draw.start, draw.end)
    : null

  return (
    <div id={`pdf-send-page-${pageNumber}`} className="relative mx-auto scroll-mt-4 shadow-md" style={{ width: renderWidth }}>
      <Page
        pageNumber={pageNumber}
        width={renderWidth}
        renderTextLayer={false}
        renderAnnotationLayer={false}
        onLoadSuccess={(page) => {
          // The frame pdf.js draws (CropBox, /Rotate) is the frame the engine
          // measures boxes in (doc-assembly#39).
          const viewport = page.getViewport({ scale: 1 })
          onPageLoad(pageNumber, {
            size: { width: viewport.width, height: viewport.height },
            // Same tolerance as the server (pdfpages), which refuses larger offsets.
            offset: Math.abs(page.view[0]) > PAGE_TOLERANCE || Math.abs(page.view[1]) > PAGE_TOLERANCE,
          })
        }}
      />
      <div
        className={cn('absolute inset-0 touch-none', activeTool && 'cursor-crosshair')}
        onPointerDown={onOverlayPointerDown}
        onPointerMove={onOverlayPointerMove}
        onPointerUp={onOverlayPointerUp}
        data-testid={`pdf-page-overlay-${pageNumber}`}
      >
        {pageSize &&
          boxes.map((box) => {
            const signerIndex = signers.findIndex((s) => s.id === box.signerId)
            const color = signerColor(signerIndex)
            const selected = box.id === selectedBoxId
            return (
              <div
                key={box.id}
                role="button"
                tabIndex={0}
                aria-label={`${fieldLabel(t, box.type)}: ${signerName(box.signerId)}`}
                aria-pressed={selected}
                className={cn(
                  'absolute cursor-move select-none border-2 px-1 font-mono text-[10px] uppercase leading-tight',
                  selected && 'z-10 ring-2 ring-foreground ring-offset-1'
                )}
                style={{
                  left: box.x * scale,
                  top: box.top * scale,
                  width: box.width * scale,
                  height: box.height * scale,
                  borderColor: color,
                  backgroundColor: `${color}26`,
                  color,
                  touchAction: 'none',
                }}
                onPointerDown={(event) =>
                  startDrag(event, { kind: 'move', boxId: box.id, startX: event.clientX, startY: event.clientY, original: box })
                }
                onPointerMove={moveDrag}
                onPointerUp={endDrag}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') onSelect(box.id)
                }}
              >
                <span className="block truncate">
                  {fieldLabel(t, box.type)} · {signerName(box.signerId)}
                </span>
                {selected &&
                  CORNERS.map(({ corner, className, cursor }) => (
                    <span
                      key={corner}
                      aria-hidden
                      data-corner={corner}
                      className={cn('absolute h-2.5 w-2.5 border border-background', className)}
                      style={{ backgroundColor: color, cursor }}
                      onPointerDown={(event) =>
                        startDrag(event, {
                          kind: 'resize',
                          corner,
                          boxId: box.id,
                          startX: event.clientX,
                          startY: event.clientY,
                          original: box,
                        })
                      }
                      onPointerMove={moveDrag}
                      onPointerUp={endDrag}
                    />
                  ))}
              </div>
            )
          })}

        {draft && (
          <div
            aria-hidden
            className="pointer-events-none absolute border-2 border-dashed"
            style={{
              left: draft.x * scale,
              top: draft.top * scale,
              width: draft.width * scale,
              height: draft.height * scale,
              borderColor: signerColor(signers.findIndex((s) => s.id === currentSignerId)),
            }}
          />
        )}

        {pageSize &&
          boxes
            .filter((box) => box.id === selectedBoxId)
            .map((box) => (
              <div
                key={`${box.id}-toolbar`}
                className="absolute z-20 flex items-center gap-1 border border-border bg-background p-1 shadow-sm"
                style={{ left: box.x * scale, top: Math.max(0, box.top * scale - 34) }}
                onPointerDown={(event) => event.stopPropagation()}
              >
                <select
                  aria-label={t('pdfSend.boxSigner', 'Signer for this box')}
                  className="h-6 border border-border bg-background px-1 text-xs"
                  value={box.signerId}
                  onChange={(event) => onChange({ ...box, signerId: event.target.value })}
                >
                  {signers.map((signer) => (
                    <option key={signer.id} value={signer.id}>
                      {signerName(signer.id)}
                    </option>
                  ))}
                </select>
                <select
                  aria-label={t('pdfSend.boxType', 'Field type for this box')}
                  className="h-6 border border-border bg-background px-1 text-xs"
                  value={box.type}
                  onChange={(event) => onChange({ ...box, type: event.target.value as FieldType })}
                >
                  {FIELD_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {fieldLabel(t, type)}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  aria-label={t('pdfSend.deleteBox', 'Delete box')}
                  className="flex h-6 w-6 items-center justify-center text-muted-foreground hover:text-destructive"
                  onClick={() => onDelete(box.id)}
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
      </div>
    </div>
  )
}
