import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Check,
  ChevronDown,
  GripVertical,
  Lock,
  MapPin,
  X,
} from 'lucide-react'
import { cn } from '@/lib/utils'

export interface SigningProgressTask {
  id: string
  label: string
  description?: string
  completed: boolean
  kind: 'interactive' | 'agreement'
}

interface FloatingSigningProgressProps {
  tasks: SigningProgressTask[]
  actionLabel: string
  expandedActionLabel: string
  onAction: () => void
  onTaskSelect: (task: SigningProgressTask) => void
  onAgreementChange?: (checked: boolean) => void
  disabledReason?: string
  loading?: boolean
}

interface Position {
  x: number
  y: number
}

type ProgressMotion = 'grow' | 'shrink' | null

const DEFAULT_POSITION: Position = { x: 58, y: 145 }

export function FloatingSigningProgress({
  tasks,
  actionLabel,
  expandedActionLabel,
  onAction,
  onTaskSelect,
  onAgreementChange,
  disabledReason,
  loading = false,
}: FloatingSigningProgressProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(true)
  const [position, setPosition] = useState<Position>(DEFAULT_POSITION)
  const [progressMotion, setProgressMotion] = useState<ProgressMotion>(null)
  const [compactActionTooltipVisible, setCompactActionTooltipVisible] = useState(false)
  const dragRef = useRef<{
    startX: number
    startY: number
    originX: number
    originY: number
  } | null>(null)

  const completedCount = tasks.filter((task) => task.completed).length
  const totalCount = Math.max(tasks.length, 1)
  const pendingCount = Math.max(totalCount - completedCount, 0)
  const allComplete = pendingCount === 0
  const progress = Math.round((completedCount / totalCount) * 100)
  const previousCompletedCountRef = useRef(completedCount)

  const summary = allComplete
    ? t('publicSigning.guidance.readyTitle')
    : t(
        pendingCount === 1
          ? 'publicSigning.guidance.missingTitleOne'
          : 'publicSigning.guidance.missingTitleOther',
        { count: pendingCount },
      )
  const expandedSummary = t('publicSigning.guidance.completedTitle', {
    completed: completedCount,
    total: totalCount,
  })
  const pendingText = t(
    pendingCount === 1
      ? 'publicSigning.guidance.pendingOne'
      : 'publicSigning.guidance.pendingOther',
    { count: pendingCount },
  )
  const counterTooltip = `${summary}. ${pendingText}`
  const interactiveTasks = tasks.filter((task) => task.kind === 'interactive')
  const agreementTask = tasks.find((task) => task.kind === 'agreement')

  const clampPosition = useCallback((next: Position): Position => {
    const width = expanded ? 426 : 360
    const height = expanded ? 520 : 72
    const padding = 16
    const maxX = Math.max(padding, window.innerWidth - width - padding)
    const maxY = Math.max(padding, window.innerHeight - height - padding)
    return {
      x: Math.min(Math.max(next.x, padding), maxX),
      y: Math.min(Math.max(next.y, padding), maxY),
    }
  }, [expanded])

  useEffect(() => {
    const onMove = (event: PointerEvent) => {
      if (!dragRef.current) return
      const dx = event.clientX - dragRef.current.startX
      const dy = event.clientY - dragRef.current.startY
      setPosition(
        clampPosition({
          x: dragRef.current.originX - dx,
          y: dragRef.current.originY + dy,
        }),
      )
    }
    const onUp = () => {
      dragRef.current = null
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [clampPosition])

  const startDrag = useCallback((event: React.PointerEvent<HTMLElement>) => {
    event.currentTarget.setPointerCapture?.(event.pointerId)
    dragRef.current = {
      startX: event.clientX,
      startY: event.clientY,
      originX: position.x,
      originY: position.y,
    }
  }, [position])

  const handleAction = useCallback(() => {
    if (!allComplete || loading) return
    onAction()
  }, [allComplete, loading, onAction])

  const handleTaskClick = useCallback(
    (task: SigningProgressTask) => {
      if (task.kind === 'agreement') return
      onTaskSelect(task)
    },
    [onTaskSelect],
  )

  const ringStyle = useMemo(
    () => ({
      background: `conic-gradient(rgb(104 224 138) ${progress}%, rgba(148, 163, 184, 0.22) 0)`,
    }),
    [progress],
  )

  useEffect(() => {
    const previousCompletedCount = previousCompletedCountRef.current
    if (previousCompletedCount === completedCount) return

    const nextMotion = completedCount > previousCompletedCount ? 'grow' : 'shrink'
    previousCompletedCountRef.current = completedCount

    let timeout: number | undefined
    const frame = window.requestAnimationFrame(() => {
      setProgressMotion(nextMotion)
      timeout = window.setTimeout(() => setProgressMotion(null), 460)
    })

    return () => {
      window.cancelAnimationFrame(frame)
      if (timeout !== undefined) {
        window.clearTimeout(timeout)
      }
    }
  }, [completedCount])

  return (
    <section
      data-testid="floating-signing-progress"
      aria-label={t('publicSigning.guidance.ariaLabel')}
      className={cn(
        'fixed z-[60] text-slate-100 transition-[width,transform] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]',
        expanded
          ? 'w-[min(426px,calc(100vw-32px))]'
          : 'w-[min(360px,calc(100vw-32px))]',
      )}
      style={{ right: position.x, top: position.y }}
    >
      <div
        className={cn(
          'rounded-[22px] border border-slate-500/30',
          'bg-[#172232]/92 shadow-[0_18px_55px_rgba(0,0,0,0.42),inset_0_1px_0_rgba(255,255,255,0.06)]',
          'backdrop-blur-xl supports-[backdrop-filter]:bg-[#172232]/80',
          'transition-[min-height,border-radius,box-shadow] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]',
          expanded ? 'min-h-[430px] overflow-hidden' : 'min-h-[72px] overflow-visible',
        )}
      >
        {expanded ? (
          <div className="flex min-h-[430px] flex-col px-8 py-5 motion-safe:animate-[floating-panel-in_220ms_ease-out]">
            <div
              data-testid="expanded-drag-surface"
              onPointerDown={startDrag}
              className="mb-5 flex cursor-grab touch-none items-center justify-between rounded-lg -mx-1 px-1 py-0.5 text-slate-400 transition-colors hover:bg-white/[0.03] hover:text-slate-100 active:cursor-grabbing"
            >
              <span
                aria-label={t('publicSigning.guidance.dragHandle')}
                className="grid h-7 w-7 place-items-center"
              >
                <GripVertical size={20} />
              </span>
              <button
                type="button"
                aria-label={t('publicSigning.guidance.collapse')}
                onClick={() => setExpanded(false)}
                onPointerDown={(event) => event.stopPropagation()}
                className="rounded-full p-1 text-slate-400 transition-colors hover:bg-white/5 hover:text-slate-100 focus:outline-none focus:ring-2 focus:ring-emerald-300/60"
              >
                <X size={18} />
              </button>
            </div>

            <div className="mb-6 flex items-center gap-4">
              <ProgressRing
                key={`expanded-progress-${completedCount}`}
                ringStyle={ringStyle}
                completed={completedCount}
                total={totalCount}
                motion={progressMotion}
              />
              <div className="min-w-0">
                <h2 className="text-[15px] font-semibold leading-tight text-white">
                  {expandedSummary}
                </h2>
                <p className="mt-1 text-sm leading-snug text-slate-300">
                  {allComplete
                    ? t('publicSigning.guidance.readyDescription')
                    : t(
                        pendingCount === 1
                          ? 'publicSigning.guidance.missingDescriptionOne'
                          : 'publicSigning.guidance.missingDescriptionOther',
                        { count: pendingCount },
                      )}
                </p>
              </div>
            </div>

            <div className="space-y-1.5">
              {interactiveTasks.map((task) => (
                <TaskRow
                  key={task.id}
                  task={task}
                  onTaskClick={handleTaskClick}
                />
              ))}
            </div>

            <div className="mt-auto space-y-5 pt-7">
              {agreementTask ? (
                <TaskRow
                  task={agreementTask}
                  onTaskClick={handleTaskClick}
                  onAgreementChange={onAgreementChange}
                />
              ) : null}

              <button
                type="button"
                onClick={handleAction}
                disabled={!allComplete || loading}
                className={cn(
                  'flex h-12 w-full items-center justify-center gap-2 rounded-md text-sm font-semibold transition-colors',
                  allComplete && !loading
                    ? 'bg-emerald-400 text-slate-950 hover:bg-emerald-300'
                    : 'pointer-events-none cursor-not-allowed bg-slate-200/12 text-slate-300/55',
                )}
              >
                <Lock size={15} />
                {loading ? t('publicSigning.submitting') : expandedActionLabel}
              </button>

              {disabledReason ? (
                <p className="flex items-center gap-2 text-xs text-slate-300">
                  <Lock size={13} />
                  <span>{disabledReason}</span>
                </p>
              ) : null}
            </div>
          </div>
        ) : (
          <div
            data-testid="compact-drag-surface"
            onPointerDown={startDrag}
            className="flex h-[72px] cursor-grab touch-none items-center gap-4 px-4 motion-safe:animate-[floating-panel-in_220ms_ease-out] active:cursor-grabbing"
          >
            <span
              aria-label={t('publicSigning.guidance.dragHandle')}
              className="grid h-7 w-5 place-items-center text-slate-400 transition-colors hover:text-slate-100"
            >
              <GripVertical size={20} />
            </span>

            <ProgressCounterWithTooltip
              ringStyle={ringStyle}
              completed={completedCount}
              total={totalCount}
              tooltip={counterTooltip}
              motion={progressMotion}
            />

            <span
              className={cn(
                'relative ml-auto hidden sm:block',
                !allComplete || loading ? 'cursor-not-allowed' : 'cursor-default',
              )}
              onPointerDown={(event) => event.stopPropagation()}
              onPointerEnter={() => setCompactActionTooltipVisible(true)}
              onPointerLeave={() => setCompactActionTooltipVisible(false)}
              onFocus={() => setCompactActionTooltipVisible(true)}
              onBlur={() => setCompactActionTooltipVisible(false)}
              tabIndex={!allComplete && disabledReason ? 0 : undefined}
            >
              <button
                type="button"
                onClick={handleAction}
                disabled={!allComplete || loading}
                aria-describedby={!allComplete && disabledReason ? 'compact-signing-action-tooltip' : undefined}
                className={cn(
                  'flex h-10 min-w-[116px] items-center justify-center gap-2 rounded-md px-3 text-sm font-semibold transition-colors',
                  allComplete && !loading
                    ? 'bg-emerald-400 text-slate-950 hover:bg-emerald-300'
                    : 'pointer-events-none cursor-not-allowed bg-slate-200/12 text-slate-300/55',
                )}
              >
                <Lock size={14} />
                {loading ? t('publicSigning.submitting') : actionLabel}
              </button>
              {!allComplete && disabledReason ? (
                <span
                  id="compact-signing-action-tooltip"
                  role="tooltip"
                  className={cn(
                    'pointer-events-none absolute left-1/2 top-[calc(100%+10px)] z-10 w-max max-w-64 -translate-x-1/2 rounded-md',
                    'border border-slate-500/30 bg-slate-950/95 px-3 py-2 text-xs leading-snug text-slate-100 shadow-xl',
                    'transition-opacity duration-150',
                    compactActionTooltipVisible ? 'opacity-100' : 'opacity-0',
                  )}
                >
                  {disabledReason}
                </span>
              ) : null}
            </span>

            <button
              type="button"
              aria-label={t('publicSigning.guidance.expand')}
              onClick={() => setExpanded(true)}
              onPointerDown={(event) => event.stopPropagation()}
              className="rounded-full p-2 text-slate-100 transition-colors hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-emerald-300/60"
            >
              <ChevronDown size={20} />
            </button>
          </div>
        )}
      </div>
      <style>
        {`
          @keyframes floating-panel-in {
            from {
              opacity: 0;
              transform: translateY(-6px) scale(0.985);
            }
            to {
              opacity: 1;
              transform: translateY(0) scale(1);
            }
          }
          @keyframes progress-ring-grow {
            0% {
              transform: scale(1);
            }
            45% {
              transform: scale(1.08);
            }
            100% {
              transform: scale(1);
            }
          }
          @keyframes progress-ring-shrink {
            0% {
              transform: scale(1);
            }
            45% {
              transform: scale(0.92);
            }
            100% {
              transform: scale(1);
            }
          }
        `}
      </style>
    </section>
  )
}

function ProgressCounterWithTooltip({
  ringStyle,
  completed,
  total,
  tooltip,
  motion,
}: {
  ringStyle: React.CSSProperties
  completed: number
  total: number
  tooltip: string
  motion?: ProgressMotion
}) {
  return (
    <div
      className="group relative shrink-0"
      title={tooltip}
      aria-label={tooltip}
      tabIndex={0}
    >
      <ProgressRing
        key={`compact-progress-${completed}`}
        ringStyle={ringStyle}
        completed={completed}
        total={total}
        size="compact"
        ariaHidden
        motion={motion}
      />
      <span
        role="tooltip"
        className={cn(
          'pointer-events-none absolute left-1/2 top-[calc(100%+10px)] z-10 w-max max-w-56 -translate-x-1/2 rounded-md',
          'border border-slate-500/30 bg-slate-950/95 px-3 py-2 text-xs leading-snug text-slate-100 shadow-xl',
          'opacity-0 transition-opacity duration-150 group-hover:opacity-100 group-focus:opacity-100',
        )}
      >
        {tooltip}
      </span>
    </div>
  )
}

function ProgressRing({
  ringStyle,
  completed,
  total,
  size = 'default',
  title,
  ariaHidden = false,
  motion,
}: {
  ringStyle: React.CSSProperties
  completed: number
  total: number
  size?: 'default' | 'compact'
  title?: string
  ariaHidden?: boolean
  motion?: ProgressMotion
}) {
  return (
    <div
      data-testid="signing-progress-ring"
      data-motion={motion ?? undefined}
      className={cn(
        'grid shrink-0 place-items-center rounded-full p-[4px] will-change-transform',
        size === 'compact' ? 'h-14 w-14' : 'h-[72px] w-[72px]',
        motion === 'grow' && 'motion-safe:animate-[progress-ring-grow_420ms_ease-out]',
        motion === 'shrink' && 'motion-safe:animate-[progress-ring-shrink_420ms_ease-out]',
      )}
      style={ringStyle}
      title={title}
      aria-label={ariaHidden ? undefined : title}
      aria-hidden={ariaHidden}
    >
      <div className="grid h-full w-full place-items-center rounded-full bg-[#172232] text-sm font-semibold text-white shadow-[inset_0_0_0_1px_rgba(255,255,255,0.06)]">
        {completed}/{total}
      </div>
    </div>
  )
}

function TaskRow({
  task,
  onTaskClick,
  onAgreementChange,
}: {
  task: SigningProgressTask
  onTaskClick: (task: SigningProgressTask) => void
  onAgreementChange?: (checked: boolean) => void
}) {
  if (task.kind === 'agreement') {
    return (
      <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md border border-white/5 bg-slate-300/7 px-4 text-sm text-slate-100 transition-colors hover:bg-slate-300/10">
        <span
          className={cn(
            'grid h-5 w-5 place-items-center rounded border',
            task.completed
              ? 'border-emerald-400 bg-emerald-400 text-slate-950'
              : 'border-slate-300/70 bg-transparent text-transparent',
          )}
          aria-hidden="true"
        >
          <Check size={14} />
        </span>
        <input
          type="checkbox"
          className="sr-only"
          checked={task.completed}
          onChange={(event) => onAgreementChange?.(event.target.checked)}
        />
        <span>{task.label}</span>
      </label>
    )
  }

  return (
    <button
      type="button"
      onClick={() => onTaskClick(task)}
      className="relative flex min-h-11 w-full items-center gap-3 rounded-md border border-white/5 bg-slate-300/7 py-1.5 pl-[10px] pr-9 text-left text-sm text-slate-100 transition-colors hover:bg-slate-300/12 focus:outline-none focus:ring-2 focus:ring-emerald-300/60"
      aria-label={task.label}
    >
      <span
        className={cn(
          'grid h-8 w-8 shrink-0 place-items-center rounded-md border transition-colors',
          task.completed
            ? 'border-emerald-300/25 bg-emerald-300/10 text-emerald-200'
            : 'border-amber-200/25 bg-amber-200/10 text-amber-100',
        )}
      >
        <MapPin size={16} aria-hidden="true" />
      </span>
      <span className="min-w-0">
        <span className="block truncate">{task.label}</span>
      </span>
    </button>
  )
}
