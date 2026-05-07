import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { FloatingSigningProgress, type SigningProgressTask } from './FloatingSigningProgress'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const translations: Record<string, string> = {
        'publicSigning.guidance.ariaLabel': 'Guía de firma',
        'publicSigning.guidance.dragHandle': 'Mover guía de firma',
        'publicSigning.guidance.expand': 'Expandir guía de firma',
        'publicSigning.guidance.collapse': 'Contraer guía de firma',
        'publicSigning.guidance.missingTitleOne': `Falta ${String(params?.count)} confirmación`,
        'publicSigning.guidance.missingTitleOther': `Faltan ${String(params?.count)} confirmaciones`,
        'publicSigning.guidance.pendingOne': `${String(params?.count)} pendiente`,
        'publicSigning.guidance.pendingOther': `${String(params?.count)} pendientes`,
        'publicSigning.guidance.completedTitle': `${String(params?.completed)} de ${String(params?.total)} confirmaciones completadas`,
        'publicSigning.guidance.missingDescriptionOne': `Te falta ${String(params?.count)} paso para habilitar la firma`,
        'publicSigning.guidance.missingDescriptionOther': `Te faltan ${String(params?.count)} pasos para habilitar la firma`,
      }
      return translations[key] ?? key
    },
  }),
}))

const tasks: SigningProgressTask[] = [
  {
    id: 'field-1',
    label: 'Autorización Clases de Religión',
    description: 'Responde para generar el contrato',
    completed: true,
    kind: 'interactive',
  },
  {
    id: 'agreement',
    label: 'He leído y revisado el documento',
    completed: false,
    kind: 'agreement',
  },
]

const completedTasks: SigningProgressTask[] = tasks.map((task) => ({
  ...task,
  completed: true,
}))

describe('FloatingSigningProgress', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('renders expanded by default and collapses to compact progress with a counter tooltip', async () => {
    const user = userEvent.setup()

    render(
      <FloatingSigningProgress
        tasks={tasks}
        actionLabel="Ir a firmar"
        expandedActionLabel="Continuar a firma"
        onAction={vi.fn()}
        onTaskSelect={vi.fn()}
        disabledReason="Completa la confirmación final para habilitar la firma"
      />,
    )

    expect(screen.getByText('1 de 2 confirmaciones completadas')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Contraer guía de firma' }))

    expect(screen.getByText('1/2')).toBeTruthy()
    expect(screen.getByText('Falta 1 confirmación. 1 pendiente')).toBeTruthy()
    expect(screen.getByLabelText('Falta 1 confirmación. 1 pendiente')).toBeTruthy()
    const disabledTooltip = screen.getByText('Completa la confirmación final para habilitar la firma')
    expect(disabledTooltip).toBeTruthy()
    expect(disabledTooltip.getAttribute('class')).toContain('opacity-0')

    const compactActionWrapper = screen.getByRole('button', { name: 'Ir a firmar' }).parentElement!
    expect(compactActionWrapper.getAttribute('class')).toContain('cursor-not-allowed')

    await user.hover(compactActionWrapper)
    expect(disabledTooltip.getAttribute('class')).toContain('opacity-100')

    expect(screen.getByRole('button', { name: 'Ir a firmar' }).hasAttribute('disabled')).toBe(true)
  })

  it('moves away from the right edge when dragged left', () => {
    render(
      <FloatingSigningProgress
        tasks={tasks}
        actionLabel="Ir a firmar"
        expandedActionLabel="Continuar a firma"
        onAction={vi.fn()}
        onTaskSelect={vi.fn()}
      />,
    )

    const bar = screen.getByTestId('floating-signing-progress')
    expect(bar.getAttribute('style')).toContain('right: 58px')

    const handle = screen.getByTestId('expanded-drag-surface')
    fireEvent.pointerDown(handle, { pointerId: 1, clientX: 760, clientY: 180 })
    fireEvent.pointerMove(window, { pointerId: 1, clientX: 660, clientY: 180 })
    fireEvent.pointerUp(window, { pointerId: 1, clientX: 660, clientY: 180 })

    expect(bar.getAttribute('style')).toContain('right: 158px')
  })

  it('expands to show task completion states and toggles agreement from the checklist', async () => {
    const user = userEvent.setup()
    const onAgreementChange = vi.fn()

    render(
      <FloatingSigningProgress
        tasks={tasks}
        actionLabel="Ir a firmar"
        expandedActionLabel="Continuar a firma"
        onAction={vi.fn()}
        onTaskSelect={vi.fn()}
        onAgreementChange={onAgreementChange}
        disabledReason="Completa la confirmación final para habilitar la firma"
      />,
    )

    expect(screen.getByText('1 de 2 confirmaciones completadas')).toBeTruthy()
    expect(screen.getByText('Autorización Clases de Religión')).toBeTruthy()
    expect(screen.queryByText('Responde para generar el contrato')).toBeNull()
    expect(screen.getByText('He leído y revisado el documento')).toBeTruthy()

    await user.click(screen.getByRole('checkbox', { name: 'He leído y revisado el documento' }))
    expect(onAgreementChange).toHaveBeenCalledWith(true)
  })

  it('scrolls the document to an interactive task when that task is selected', async () => {
    const user = userEvent.setup()
    const onTaskSelect = vi.fn()

    render(
      <FloatingSigningProgress
        tasks={tasks}
        actionLabel="Ir a firmar"
        expandedActionLabel="Continuar a firma"
        onAction={vi.fn()}
        onTaskSelect={onTaskSelect}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Autorización Clases de Religión' }))

    expect(onTaskSelect).toHaveBeenCalledWith(tasks[0])
  })

  it('keeps the final confirmation helper visible after every task is complete', () => {
    render(
      <FloatingSigningProgress
        tasks={completedTasks}
        actionLabel="Ir a firmar"
        expandedActionLabel="Continuar a firma"
        onAction={vi.fn()}
        onTaskSelect={vi.fn()}
        disabledReason="Completa la confirmación final para habilitar la firma"
      />,
    )

    expect(screen.getByText('Completa la confirmación final para habilitar la firma')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Continuar a firma' }).hasAttribute('disabled')).toBe(false)
  })
})
