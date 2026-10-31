import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, GripVertical, Plus, Trash2 } from 'lucide-react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'

import { cn } from '@/lib/utils'

import { fieldLabel } from '../field-label'
import type { Box, Signer } from '../model'
import { signerColor } from '../signer-colors'

interface SignersPanelProps {
  signers: Signer[]
  boxes: Box[]
  currentSignerId: string
  onSelectSigner: (signerId: string) => void
  onAdd: () => void
  onRemove: (signerId: string) => void
  onUpdate: (signerId: string, patch: Partial<Signer>) => void
  onMove: (from: number, to: number) => void
  onJumpToBox: (box: Box) => void
}

/**
 * The signers, in signing order: the row number is the order. Rows reorder by
 * dragging the handle (or with the keyboard, or the arrows). The selected row
 * is the signer new boxes go to. Under each signer, their boxes; clicking one
 * jumps to its page.
 */
export function SignersPanel({
  signers,
  boxes,
  currentSignerId,
  onSelectSigner,
  onAdd,
  onRemove,
  onUpdate,
  onMove,
  onJumpToBox,
}: SignersPanelProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const from = signers.findIndex((s) => s.id === active.id)
    const to = signers.findIndex((s) => s.id === over.id)
    if (from >= 0 && to >= 0) onMove(from, to)
  }

  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
          {t('pdfSend.signers', 'Signers, in signing order')}
        </span>
        <button type="button" onClick={onAdd} className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <Plus size={12} />
          {t('pdfSend.addSigner', 'Add signer')}
        </button>
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={signers.map((s) => s.id)} strategy={verticalListSortingStrategy}>
          <ol className="space-y-2">
            {signers.map((signer, index) => (
              <SignerRow
                key={signer.id}
                signer={signer}
                index={index}
                count={signers.length}
                boxes={boxes.filter((b) => b.signerId === signer.id).sort((a, b) => a.page - b.page || a.top - b.top)}
                selected={signer.id === currentSignerId}
                onSelect={() => onSelectSigner(signer.id)}
                onRemove={() => onRemove(signer.id)}
                onUpdate={(patch) => onUpdate(signer.id, patch)}
                onMove={(to) => onMove(index, to)}
                onJumpToBox={onJumpToBox}
              />
            ))}
          </ol>
        </SortableContext>
      </DndContext>
    </section>
  )
}

interface SignerRowProps {
  signer: Signer
  index: number
  count: number
  boxes: Box[]
  selected: boolean
  onSelect: () => void
  onRemove: () => void
  onUpdate: (patch: Partial<Signer>) => void
  onMove: (to: number) => void
  onJumpToBox: (box: Box) => void
}

function SignerRow({ signer, index, count, boxes, selected, onSelect, onRemove, onUpdate, onMove, onJumpToBox }: SignerRowProps) {
  const { t } = useTranslation()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: signer.id })
  const n = index + 1

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn('space-y-1 border bg-background p-2', selected ? 'border-foreground' : 'border-border', isDragging && 'opacity-60')}
      onClick={onSelect}
      data-testid={`signer-row-${n}`}
    >
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="cursor-grab text-muted-foreground hover:text-foreground"
          aria-label={t('pdfSend.dragSigner', 'Drag to reorder signer {{n}}', { n })}
          {...attributes}
          {...listeners}
        >
          <GripVertical size={14} />
        </button>
        <span className="h-3 w-3 shrink-0" style={{ backgroundColor: signerColor(index) }} aria-hidden />
        <span className="font-mono text-[10px] text-muted-foreground">{n}</span>
        <input
          aria-label={t('pdfSend.signerName', 'Signer {{n}} name', { n })}
          placeholder={t('pdfSend.name', 'Name')}
          value={signer.name}
          onChange={(event) => onUpdate({ name: event.target.value })}
          className="h-8 min-w-0 flex-1 border border-border bg-background px-2 text-sm"
        />
        <button
          type="button"
          aria-label={t('pdfSend.moveUp', 'Move up')}
          disabled={index === 0}
          onClick={(event) => {
            event.stopPropagation()
            onMove(index - 1)
          }}
          className="text-muted-foreground hover:text-foreground disabled:opacity-30"
        >
          <ArrowUp size={14} />
        </button>
        <button
          type="button"
          aria-label={t('pdfSend.moveDown', 'Move down')}
          disabled={index === count - 1}
          onClick={(event) => {
            event.stopPropagation()
            onMove(index + 1)
          }}
          className="text-muted-foreground hover:text-foreground disabled:opacity-30"
        >
          <ArrowDown size={14} />
        </button>
        <button
          type="button"
          aria-label={t('pdfSend.removeSigner', 'Remove signer')}
          disabled={count === 1}
          onClick={(event) => {
            event.stopPropagation()
            onRemove()
          }}
          className="text-muted-foreground hover:text-destructive disabled:opacity-30"
        >
          <Trash2 size={14} />
        </button>
      </div>
      <input
        type="email"
        aria-label={t('pdfSend.signerEmail', 'Signer {{n}} email', { n })}
        placeholder={t('pdfSend.email', 'Email')}
        value={signer.email}
        onChange={(event) => onUpdate({ email: event.target.value })}
        className="h-8 w-full border border-border bg-background px-2 text-sm"
      />
      {boxes.length > 0 && (
        <ul className="flex flex-wrap gap-1 pt-1">
          {boxes.map((box) => (
            <li key={box.id}>
              <button
                type="button"
                onClick={(event) => {
                  event.stopPropagation()
                  onJumpToBox(box)
                }}
                className="border border-border px-1.5 py-0.5 font-mono text-[10px] uppercase text-muted-foreground hover:border-foreground hover:text-foreground"
              >
                {fieldLabel(t, box.type)} · {t('pdfSend.pageShort', 'p.{{n}}', { n: box.page })}
              </button>
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}
