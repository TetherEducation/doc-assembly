import { useTranslation } from 'react-i18next'
import type { TemplateVersionInjectable } from '@/types/api'
import { resolveI18n } from '@/features/editor/types/i18n-resolve'

interface WizardStepValuesProps {
  injectables: TemplateVersionInjectable[]
  values: Record<string, unknown>
  onValuesChange: (values: Record<string, unknown>) => void
}

export function WizardStepValues({
  injectables,
  values,
  onValuesChange,
}: WizardStepValuesProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language.split('-')[0]

  if (injectables.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        {t(
          'signing.wizard.noInjectables',
          'This template version has no injectable fields.'
        )}
      </p>
    )
  }

  const handleChange = (key: string, value: unknown) => {
    onValuesChange({ ...values, [key]: value })
  }

  return (
    <div className="space-y-5">
      {injectables.map((injectable) => {
        const { definition, isRequired, defaultValue } = injectable
        // The API sends label/description as i18n maps; workspace fill-ins
        // arrive as {"_": text}. Rendering the map itself crashes React (#31).
        const label =
          resolveI18n(definition.label, locale, definition.key) || definition.key
        const description = resolveI18n(definition.description, locale)
        const currentValue =
          (values[definition.key] as string) ?? defaultValue ?? ''

        return (
          <div key={injectable.id}>
            <label
              htmlFor={`injectable-${definition.key}`}
              className="mb-2 block font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground"
            >
              {label}
              {isRequired && (
                <span className="ml-1 text-destructive">*</span>
              )}
            </label>
            {description && (
              <p className="mb-1 text-xs text-muted-foreground/70">
                {description}
              </p>
            )}
            {definition.dataType === 'BOOLEAN' ? (
              <label className="flex items-center gap-2 py-2 text-sm font-light">
                <input
                  type="checkbox"
                  checked={currentValue === true || currentValue === 'true'}
                  onChange={(e) =>
                    handleChange(definition.key, e.target.checked)
                  }
                  className="h-4 w-4"
                />
                {label}
              </label>
            ) : (
              <input
                id={`injectable-${definition.key}`}
                type={
                  definition.dataType === 'NUMBER' ||
                  definition.dataType === 'CURRENCY'
                    ? 'number'
                    : definition.dataType === 'DATE'
                      ? 'date'
                      : 'text'
                }
                value={String(currentValue)}
                onChange={(e) => handleChange(definition.key, e.target.value)}
                required={isRequired}
                className="w-full rounded-none border-0 border-b border-border bg-transparent py-2 text-base font-light text-foreground outline-none transition-all placeholder:text-muted-foreground/50 focus-visible:border-foreground focus-visible:ring-0"
                placeholder={label}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}
