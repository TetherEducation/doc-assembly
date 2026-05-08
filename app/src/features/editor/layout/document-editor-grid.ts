import {
  PANEL_COLLAPSED_WIDTH,
  ROLES_EXPANDED_WIDTH,
  VARIABLES_EXPANDED_WIDTH,
} from './panel-widths'

export const DOCUMENT_EDITOR_GRID_BASE_CLASS =
  'grid grid-rows-[auto_1fr] h-full w-full min-w-0 overflow-hidden transition-[grid-template-columns] duration-200 ease-[cubic-bezier(0.4,0,0.2,1)]'

export const DOCUMENT_EDITOR_GRID_EDITABLE_CLASS =
  'grid-cols-[auto_minmax(0,1fr)_auto]'

export const DOCUMENT_EDITOR_GRID_READ_ONLY_CLASS =
  'grid-cols-[minmax(0,1fr)_auto]'

export const DOCUMENT_EDITOR_GRID_EDITABLE_NO_ROLES_CLASS =
  'grid-cols-[auto_minmax(0,1fr)]'

export const DOCUMENT_EDITOR_GRID_READ_ONLY_NO_ROLES_CLASS =
  'grid-cols-[minmax(0,1fr)]'

export function getDocumentEditorGridClass(
  editable: boolean,
  showSignerRolesPanel = true
): string {
  const modeClass = editable
    ? showSignerRolesPanel
      ? DOCUMENT_EDITOR_GRID_EDITABLE_CLASS
      : DOCUMENT_EDITOR_GRID_EDITABLE_NO_ROLES_CLASS
    : showSignerRolesPanel
      ? DOCUMENT_EDITOR_GRID_READ_ONLY_CLASS
      : DOCUMENT_EDITOR_GRID_READ_ONLY_NO_ROLES_CLASS

  return [
    DOCUMENT_EDITOR_GRID_BASE_CLASS,
    modeClass,
  ].join(' ')
}

interface DocumentEditorGridTemplateColumnsParams {
  editable: boolean
  variablesCollapsed: boolean
  rolesCollapsed: boolean
  showSignerRolesPanel?: boolean
}

const CENTER_COLUMN = 'minmax(0,1fr)'

export function getDocumentEditorGridTemplateColumns({
  editable,
  variablesCollapsed,
  rolesCollapsed,
  showSignerRolesPanel = true,
}: DocumentEditorGridTemplateColumnsParams): string {
  const rolesWidth = rolesCollapsed ? PANEL_COLLAPSED_WIDTH : ROLES_EXPANDED_WIDTH

  if (!editable) {
    if (!showSignerRolesPanel) {
      return CENTER_COLUMN
    }
    return `${CENTER_COLUMN} ${rolesWidth}px`
  }

  const variablesWidth = variablesCollapsed
    ? PANEL_COLLAPSED_WIDTH
    : VARIABLES_EXPANDED_WIDTH

  if (!showSignerRolesPanel) {
    return `${variablesWidth}px ${CENTER_COLUMN}`
  }

  return `${variablesWidth}px ${CENTER_COLUMN} ${rolesWidth}px`
}
