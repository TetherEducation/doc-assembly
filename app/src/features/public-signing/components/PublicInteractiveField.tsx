import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { NodeViewWrapper, type NodeViewProps } from '@tiptap/react'
import { CheckSquare, Circle, Type, FormInput } from 'lucide-react'
import { cn } from '@/lib/utils'
import type {
  InteractiveFieldOption,
  FieldResponses,
} from '../types'

type InteractiveFieldType = 'checkbox' | 'radio' | 'text'

interface PublicFieldRuntimeState {
  responses: FieldResponses
  validationErrors: Set<string>
  submitted: boolean
}

interface PublicFieldContext {
  /** The role ID of the current signer */
  signerRoleId: string
  /** Explicit field IDs owned by this signer according to backend */
  allowedFieldIdsRef: RefObject<Set<string>>
  /** Latest runtime state without recreating node views */
  runtimeStateRef: RefObject<PublicFieldRuntimeState>
  /** Update a single field response */
  onResponseChange: (
    fieldId: string,
    fieldType: string,
    response: { selectedOptionIds?: string[]; text?: string },
  ) => void
}

const fieldTypeIcons: Record<InteractiveFieldType, typeof CheckSquare> = {
  checkbox: CheckSquare,
  radio: Circle,
  text: Type,
}

interface PublicFieldOwnershipInput {
  roleId: string
  signerRoleId: string
  hasAllowedFieldAccess: boolean
}

export function isPublicFieldOwned(input: PublicFieldOwnershipInput): boolean {
  return input.hasAllowedFieldAccess || input.roleId === input.signerRoleId
}

/**
 * Public-facing variant of InteractiveFieldComponent.
 *
 * Instead of an editor placeholder, this renders actual form inputs:
 *  - checkbox: real `<input type="checkbox">` elements
 *  - radio: real `<input type="radio">` elements
 *  - text: real `<input>` or `<textarea>` elements
 *
 * Fields belonging to another role are rendered as disabled.
 */
export function createPublicInteractiveFieldComponent(ctx: PublicFieldContext) {
  return function PublicInteractiveField(props: NodeViewProps) {
    const { node } = props
    const { t } = useTranslation()

    const fieldId = node.attrs.id as string
    const fieldType = (node.attrs.fieldType ?? 'checkbox') as InteractiveFieldType
    const roleId = (node.attrs.roleId ?? '') as string
    const label = (node.attrs.label ?? '') as string
    const required = (node.attrs.required ?? false) as boolean
    const signerRoleId = ctx.signerRoleId
    const hasFallbackOwnership = ctx.allowedFieldIdsRef.current?.has(fieldId) ?? false
    const options = useMemo(
      () => (node.attrs.options ?? []) as InteractiveFieldOption[],
      [node.attrs.options],
    )
    const placeholder = (node.attrs.placeholder ?? '') as string
    const maxLength = (node.attrs.maxLength ?? 0) as number
    const runtimeState = ctx.runtimeStateRef.current ?? {
      responses: {},
      validationErrors: new Set<string>(),
      submitted: false,
    }
    const { responses, validationErrors, submitted } = runtimeState

    const isOwnField = isPublicFieldOwned({
      roleId,
      signerRoleId,
      hasAllowedFieldAccess: hasFallbackOwnership,
    })
    const disabled = !isOwnField
    const currentResponse = responses[fieldId]
    const hasError =
      submitted && validationErrors.has(fieldId)
    const warnedMismatchRef = useRef(false)
    const [localSelectedIds, setLocalSelectedIds] = useState<string[]>(
      () => currentResponse?.response.selectedOptionIds ?? [],
    )
    const [localTextValue, setLocalTextValue] = useState<string>(
      () => currentResponse?.response.text ?? '',
    )

    const responseSelectedIds = useMemo(
      () => currentResponse?.response.selectedOptionIds ?? [],
      [currentResponse],
    )
    const responseTextValue = currentResponse?.response.text ?? ''
    const selectedDeps = responseSelectedIds.join('|')

    useEffect(() => {
      setLocalSelectedIds(responseSelectedIds)
    }, [fieldId, responseSelectedIds, selectedDeps])

    useEffect(() => {
      setLocalTextValue(responseTextValue)
    }, [fieldId, responseTextValue])

    useEffect(() => {
      if (
        hasFallbackOwnership &&
        roleId &&
        roleId !== signerRoleId &&
        !warnedMismatchRef.current
      ) {
        warnedMismatchRef.current = true
        console.warn(
          '[public-signing] interactive field role mismatch resolved with allowedFieldIds fallback',
          {
            fieldId,
            fieldRoleId: roleId,
            signerRoleId,
          },
        )
      }
    }, [hasFallbackOwnership, fieldId, roleId, signerRoleId])

    const Icon = fieldTypeIcons[fieldType] || CheckSquare

    // --- Checkbox handlers ---
    const handleCheckboxChange = useCallback(
      (optionId: string, checked: boolean) => {
        const current = localSelectedIds
        const next = checked
          ? [...current, optionId]
          : current.filter((id) => id !== optionId)
        setLocalSelectedIds(next)
        ctx.onResponseChange(fieldId, fieldType, {
          selectedOptionIds: next,
        })
      },
      [fieldId, fieldType, localSelectedIds],
    )

    // --- Radio handler ---
    const handleRadioChange = useCallback(
      (optionId: string) => {
        setLocalSelectedIds([optionId])
        ctx.onResponseChange(fieldId, fieldType, {
          selectedOptionIds: [optionId],
        })
      },
      [fieldId, fieldType],
    )

    // --- Text handler ---
    const handleTextChange = useCallback(
      (value: string) => {
        setLocalTextValue(value)
        ctx.onResponseChange(fieldId, fieldType, { text: value })
      },
      [fieldId, fieldType],
    )

    const selectedIds = localSelectedIds
    const textValue = localTextValue

    // Max length validation message
    const isOverMaxLength =
      fieldType === 'text' && maxLength > 0 && textValue.length > maxLength

    const renderInputs = () => {
      if (fieldType === 'checkbox') {
        if (options.length === 0) {
          return (
            <p className="text-sm text-muted-foreground italic">
              {t('publicSigning.noOptions')}
            </p>
          )
        }
        return (
          <div className="flex flex-col gap-2">
            {options.map((opt) => (
              <label
                key={opt.id}
                className={cn(
                  'flex items-center gap-2.5 text-sm cursor-pointer select-none',
                  disabled && 'opacity-50 cursor-not-allowed',
                )}
              >
                <input
                  type="checkbox"
                  checked={selectedIds.includes(opt.id)}
                  disabled={disabled}
                  onChange={(e) =>
                    handleCheckboxChange(opt.id, e.target.checked)
                  }
                  className="h-4 w-4 rounded border-border text-primary accent-primary focus:ring-primary"
                />
                <span>{opt.label || t('publicSigning.untitledOption')}</span>
              </label>
            ))}
          </div>
        )
      }

      if (fieldType === 'radio') {
        if (options.length === 0) {
          return (
            <p className="text-sm text-muted-foreground italic">
              {t('publicSigning.noOptions')}
            </p>
          )
        }
        return (
          <div className="flex flex-col gap-2">
            {options.map((opt) => (
              <label
                key={opt.id}
                className={cn(
                  'flex items-center gap-2.5 text-sm cursor-pointer select-none',
                  disabled && 'opacity-50 cursor-not-allowed',
                )}
              >
                <input
                  type="radio"
                  name={`field-${fieldId}`}
                  checked={selectedIds.includes(opt.id)}
                  disabled={disabled}
                  onChange={() => handleRadioChange(opt.id)}
                  className="h-4 w-4 border-border text-primary accent-primary focus:ring-primary"
                />
                <span>{opt.label || t('publicSigning.untitledOption')}</span>
              </label>
            ))}
          </div>
        )
      }

      // text field
      const useTextarea = maxLength === 0 || maxLength > 100

      return (
        <div className="space-y-1">
          {useTextarea ? (
            <textarea
              value={textValue}
              disabled={disabled}
              onChange={(e) => handleTextChange(e.target.value)}
              placeholder={placeholder || t('publicSigning.textPlaceholder')}
              rows={3}
              maxLength={maxLength > 0 ? maxLength : undefined}
              className={cn(
                'w-full rounded-md border px-3 py-2 text-sm bg-background text-foreground',
                'placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary',
                'disabled:opacity-50 disabled:cursor-not-allowed resize-y',
                hasError || isOverMaxLength
                  ? 'border-destructive'
                  : 'border-border',
              )}
            />
          ) : (
            <input
              type="text"
              value={textValue}
              disabled={disabled}
              onChange={(e) => handleTextChange(e.target.value)}
              placeholder={placeholder || t('publicSigning.textPlaceholder')}
              maxLength={maxLength > 0 ? maxLength : undefined}
              className={cn(
                'w-full rounded-md border px-3 py-2 text-sm bg-background text-foreground',
                'placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary/50 focus:border-primary',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                hasError || isOverMaxLength
                  ? 'border-destructive'
                  : 'border-border',
              )}
            />
          )}
          {maxLength > 0 && !disabled && (
            <p
              className={cn(
                'text-xs',
                isOverMaxLength
                  ? 'text-destructive'
                  : 'text-muted-foreground',
              )}
            >
              {textValue.length}/{maxLength}
            </p>
          )}
        </div>
      )
    }

    return (
      <NodeViewWrapper
        className="my-4 scroll-mt-28"
        id={`public-field-${fieldId}`}
        data-public-field-id={fieldId}
      >
        <div
          contentEditable={false}
          className={cn(
            'relative w-full p-4 border-2 rounded-lg transition-colors select-none',
            disabled
              ? 'bg-muted/30 border-border/50 opacity-60'
              : hasError
                ? 'bg-destructive/5 border-destructive/40'
                : 'bg-card border-border',
          )}
        >
          {/* Tab label */}
          <div className="absolute -top-3 left-4 z-10">
            <div
              className={cn(
                'px-2 h-6 bg-card flex items-center gap-1.5 text-xs font-medium border rounded shadow-sm',
                disabled
                  ? 'text-muted-foreground border-border'
                  : 'text-primary border-primary/50',
              )}
            >
              <FormInput className="h-3.5 w-3.5" />
              <span>{t('publicSigning.interactiveField')}</span>
            </div>
          </div>

          {/* Content */}
          <div className="flex items-start gap-3 pt-1">
            <div className="flex-shrink-0 mt-0.5">
              <Icon className="h-4 w-4 text-muted-foreground" />
            </div>

            <div className="flex-1 min-w-0">
              {/* Label + required badge */}
              <div className="flex items-center gap-2 flex-wrap mb-2">
                <p className="text-sm font-medium text-foreground">
                  {label || t('publicSigning.noLabel')}
                </p>
                {required && (
                  <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                    {t('publicSigning.required')}
                  </span>
                )}
                {disabled && (
                  <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground border border-border">
                    {t('publicSigning.otherSigner')}
                  </span>
                )}
              </div>

              {/* Inputs */}
              {renderInputs()}

              {/* Validation error */}
              {hasError && !disabled && (
                <p className="text-xs text-destructive mt-1">
                  {t('publicSigning.fieldRequired')}
                </p>
              )}
            </div>
          </div>
        </div>
      </NodeViewWrapper>
    )
  }
}
