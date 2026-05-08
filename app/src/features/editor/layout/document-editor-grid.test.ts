import { describe, expect, it } from 'vitest'
import {
  DOCUMENT_EDITOR_GRID_BASE_CLASS,
  DOCUMENT_EDITOR_GRID_EDITABLE_CLASS,
  DOCUMENT_EDITOR_GRID_EDITABLE_NO_ROLES_CLASS,
  DOCUMENT_EDITOR_GRID_READ_ONLY_CLASS,
  DOCUMENT_EDITOR_GRID_READ_ONLY_NO_ROLES_CLASS,
  getDocumentEditorGridClass,
  getDocumentEditorGridTemplateColumns,
} from './document-editor-grid'
import {
  PANEL_COLLAPSED_WIDTH,
  ROLES_EXPANDED_WIDTH,
  VARIABLES_EXPANDED_WIDTH,
} from './panel-widths'

describe('getDocumentEditorGridClass', () => {
  it('uses minmax center column for editable mode', () => {
    const className = getDocumentEditorGridClass(true)

    expect(className).toContain(DOCUMENT_EDITOR_GRID_BASE_CLASS)
    expect(className).toContain(DOCUMENT_EDITOR_GRID_EDITABLE_CLASS)
    expect(className).toContain('w-full')
    expect(className).toContain('min-w-0')
    expect(className).toContain('overflow-hidden')
    expect(className).toContain('transition-[grid-template-columns]')
  })

  it('uses minmax center column for read-only mode', () => {
    const className = getDocumentEditorGridClass(false)

    expect(className).toContain(DOCUMENT_EDITOR_GRID_BASE_CLASS)
    expect(className).toContain(DOCUMENT_EDITOR_GRID_READ_ONLY_CLASS)
    expect(className).toContain('w-full')
    expect(className).toContain('min-w-0')
    expect(className).toContain('overflow-hidden')
    expect(className).toContain('transition-[grid-template-columns]')
  })

  it('uses a single center column for read-only mode without signer roles panel', () => {
    const className = getDocumentEditorGridClass(false, false)

    expect(className).toContain(DOCUMENT_EDITOR_GRID_BASE_CLASS)
    expect(className).toContain(DOCUMENT_EDITOR_GRID_READ_ONLY_NO_ROLES_CLASS)
    expect(className).not.toContain(DOCUMENT_EDITOR_GRID_READ_ONLY_CLASS)
  })

  it('uses variables and center columns for editable mode without signer roles panel', () => {
    const className = getDocumentEditorGridClass(true, false)

    expect(className).toContain(DOCUMENT_EDITOR_GRID_BASE_CLASS)
    expect(className).toContain(DOCUMENT_EDITOR_GRID_EDITABLE_NO_ROLES_CLASS)
    expect(className).not.toContain(DOCUMENT_EDITOR_GRID_EDITABLE_CLASS)
  })
})

describe('getDocumentEditorGridTemplateColumns', () => {
  it('uses expanded widths when editable and both panels are expanded', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: true,
      variablesCollapsed: false,
      rolesCollapsed: false,
    })

    expect(template).toBe(
      `${VARIABLES_EXPANDED_WIDTH}px minmax(0,1fr) ${ROLES_EXPANDED_WIDTH}px`
    )
  })

  it('collapses variables width and keeps roles expanded when editable', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: true,
      variablesCollapsed: true,
      rolesCollapsed: false,
    })

    expect(template).toBe(
      `${PANEL_COLLAPSED_WIDTH}px minmax(0,1fr) ${ROLES_EXPANDED_WIDTH}px`
    )
  })

  it('collapses both panel widths when editable and both are collapsed', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: true,
      variablesCollapsed: true,
      rolesCollapsed: true,
    })

    expect(template).toBe(
      `${PANEL_COLLAPSED_WIDTH}px minmax(0,1fr) ${PANEL_COLLAPSED_WIDTH}px`
    )
  })

  it('uses read-only center and expanded roles width when read-only', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: false,
      variablesCollapsed: false,
      rolesCollapsed: false,
    })

    expect(template).toBe(`minmax(0,1fr) ${ROLES_EXPANDED_WIDTH}px`)
  })

  it('uses read-only center and collapsed roles width when roles are collapsed', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: false,
      variablesCollapsed: false,
      rolesCollapsed: true,
    })

    expect(template).toBe(`minmax(0,1fr) ${PANEL_COLLAPSED_WIDTH}px`)
  })

  it('uses only the center column when read-only signer roles panel is hidden', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: false,
      variablesCollapsed: false,
      rolesCollapsed: false,
      showSignerRolesPanel: false,
    })

    expect(template).toBe('minmax(0,1fr)')
  })

  it('uses variables and center columns when editable signer roles panel is hidden', () => {
    const template = getDocumentEditorGridTemplateColumns({
      editable: true,
      variablesCollapsed: false,
      rolesCollapsed: false,
      showSignerRolesPanel: false,
    })

    expect(template).toBe(`${VARIABLES_EXPANDED_WIDTH}px minmax(0,1fr)`)
  })
})
