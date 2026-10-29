import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { WizardStepValues } from './WizardStepValues'
import type { TemplateVersionInjectable } from '@/types/api'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string) => fallback,
    i18n: { language: 'en' },
  }),
}))

// Shape the engine returns for a workspace fill-in (injectable_mapper.go:37-41).
const workspaceFillIn: TemplateVersionInjectable = {
  id: 'tvi-1',
  templateVersionId: 'v-1',
  isRequired: true,
  createdAt: '2026-10-11T00:00:00Z',
  definition: {
    id: 'inj-1',
    key: 'counterparty_name',
    label: { _: 'Counterparty name' },
    description: { _: 'Legal name of the other party' },
    dataType: 'TEXT',
    sourceType: 'EXTERNAL',
    isGlobal: false,
    createdAt: '2026-10-11T00:00:00Z',
  },
}

describe('WizardStepValues', () => {
  it('renders {"_": text} label maps as text instead of crashing (React #31)', () => {
    render(
      <WizardStepValues
        injectables={[workspaceFillIn]}
        values={{}}
        onValuesChange={vi.fn()}
      />,
    )
    expect(screen.getByText('Counterparty name')).toBeTruthy()
    expect(screen.getByText('Legal name of the other party')).toBeTruthy()
    expect(screen.getByPlaceholderText('Counterparty name')).toBeTruthy()
  })

  it('prefers the UI locale for system injectables', () => {
    const system = {
      ...workspaceFillIn,
      definition: {
        ...workspaceFillIn.definition,
        label: { en: 'Date', es: 'Fecha' },
        description: undefined,
      },
    }
    render(
      <WizardStepValues injectables={[system]} values={{}} onValuesChange={vi.fn()} />,
    )
    expect(screen.getByText('Date')).toBeTruthy()
  })
})
