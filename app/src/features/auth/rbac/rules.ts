// --- System Roles ---
export const SystemRole = {
  SUPERADMIN: 'SUPERADMIN',
  PLATFORM_ADMIN: 'PLATFORM_ADMIN',
} as const
export type SystemRole = (typeof SystemRole)[keyof typeof SystemRole]

// --- Tenant Roles ---
export const TenantRole = {
  OWNER: 'TENANT_OWNER',
  ADMIN: 'TENANT_ADMIN',
} as const
export type TenantRole = (typeof TenantRole)[keyof typeof TenantRole]

// --- Workspace Roles ---
export const WorkspaceRole = {
  OWNER: 'OWNER',
  ADMIN: 'ADMIN',
  EDITOR: 'EDITOR',
  OPERATOR: 'OPERATOR',
  VIEWER: 'VIEWER',
} as const
export type WorkspaceRole = (typeof WorkspaceRole)[keyof typeof WorkspaceRole]

// --- Permissions Definition ---
export const Permission = {
  // System/Admin Console Access
  ADMIN_ACCESS: 'admin:access',
  SYSTEM_TENANTS_VIEW: 'system:tenants:view',
  SYSTEM_TENANTS_MANAGE: 'system:tenants:manage',
  SYSTEM_USERS_VIEW: 'system:users:view',
  SYSTEM_USERS_MANAGE: 'system:users:manage',
  SYSTEM_SETTINGS_VIEW: 'system:settings:view',
  SYSTEM_SETTINGS_MANAGE: 'system:settings:manage',
  SYSTEM_AUDIT_VIEW: 'system:audit:view',
  SYSTEM_INJECTABLES_VIEW: 'system:injectables:view',
  SYSTEM_INJECTABLES_MANAGE: 'system:injectables:manage',
  SYSTEM_API_KEYS_MANAGE: 'system:api_keys:manage',

  // Workspace Management
  WORKSPACE_VIEW: 'workspace:view',
  WORKSPACE_UPDATE: 'workspace:update',
  WORKSPACE_ARCHIVE: 'workspace:archive',

  // Member Management
  MEMBERS_VIEW: 'members:view',
  MEMBERS_INVITE: 'members:invite',
  MEMBERS_REMOVE: 'members:remove',
  MEMBERS_UPDATE_ROLE: 'members:update_role',

  // Content Management
  CONTENT_VIEW: 'content:view',
  CONTENT_CREATE: 'content:create',
  CONTENT_EDIT: 'content:edit',
  CONTENT_DELETE: 'content:delete',

  // Template Versioning
  VERSION_VIEW: 'version:view',
  VERSION_CREATE: 'version:create',
  VERSION_EDIT_DRAFT: 'version:edit_draft',
  VERSION_DELETE_DRAFT: 'version:delete_draft',
  VERSION_PUBLISH: 'version:publish',

  // Tenant Management
  TENANT_CREATE: 'tenant:create',
  TENANT_MANAGE_SETTINGS: 'tenant:manage_settings',
  TENANT_MANAGE_WORKSPACES: 'tenant:manage_workspaces',

  // Injectable Management
  INJECTABLE_VIEW: 'injectable:view',
  INJECTABLE_CREATE: 'injectable:create',
  INJECTABLE_EDIT: 'injectable:edit',
  INJECTABLE_DELETE: 'injectable:delete',
  INJECTABLE_TOGGLE_STATUS: 'injectable:toggle_status',

  // Document Signing
  DOCUMENT_VIEW: 'document:view',
  DOCUMENT_CREATE: 'document:create',
  DOCUMENT_CANCEL: 'document:cancel',
  DOCUMENT_DEPRECATE: 'document:deprecate',
  DOCUMENT_REFRESH: 'document:refresh',
} as const
export type Permission = (typeof Permission)[keyof typeof Permission]

// --- Permission Rules ---

const COMMON_CONTENT_READ: Permission[] = [
  Permission.WORKSPACE_VIEW,
  Permission.MEMBERS_VIEW,
  Permission.CONTENT_VIEW,
  Permission.VERSION_VIEW,
]

export const WORKSPACE_RULES: Record<WorkspaceRole, Permission[]> = {
  [WorkspaceRole.OWNER]: [
    ...COMMON_CONTENT_READ,
    Permission.WORKSPACE_UPDATE,
    Permission.WORKSPACE_ARCHIVE,
    Permission.MEMBERS_INVITE,
    Permission.MEMBERS_REMOVE,
    Permission.MEMBERS_UPDATE_ROLE,
    Permission.CONTENT_CREATE,
    Permission.CONTENT_EDIT,
    Permission.CONTENT_DELETE,
    Permission.VERSION_CREATE,
    Permission.VERSION_EDIT_DRAFT,
    Permission.VERSION_DELETE_DRAFT,
    Permission.VERSION_PUBLISH,
    Permission.INJECTABLE_VIEW,
    Permission.INJECTABLE_CREATE,
    Permission.INJECTABLE_EDIT,
    Permission.INJECTABLE_DELETE,
    Permission.INJECTABLE_TOGGLE_STATUS,
    Permission.DOCUMENT_VIEW,
    Permission.DOCUMENT_CREATE,
    Permission.DOCUMENT_CANCEL,
    Permission.DOCUMENT_DEPRECATE,
    Permission.DOCUMENT_REFRESH,
  ],
  [WorkspaceRole.ADMIN]: [
    ...COMMON_CONTENT_READ,
    Permission.WORKSPACE_UPDATE,
    Permission.MEMBERS_INVITE,
    Permission.MEMBERS_REMOVE,
    Permission.CONTENT_CREATE,
    Permission.CONTENT_EDIT,
    Permission.CONTENT_DELETE,
    Permission.VERSION_CREATE,
    Permission.VERSION_EDIT_DRAFT,
    Permission.VERSION_DELETE_DRAFT,
    Permission.VERSION_PUBLISH,
    Permission.INJECTABLE_VIEW,
    Permission.INJECTABLE_CREATE,
    Permission.INJECTABLE_EDIT,
    Permission.INJECTABLE_DELETE,
    Permission.INJECTABLE_TOGGLE_STATUS,
    Permission.DOCUMENT_VIEW,
    Permission.DOCUMENT_CREATE,
    Permission.DOCUMENT_CANCEL,
    Permission.DOCUMENT_DEPRECATE,
    Permission.DOCUMENT_REFRESH,
  ],
  [WorkspaceRole.EDITOR]: [
    ...COMMON_CONTENT_READ,
    Permission.CONTENT_CREATE,
    Permission.CONTENT_EDIT,
    Permission.VERSION_CREATE,
    Permission.VERSION_EDIT_DRAFT,
    Permission.INJECTABLE_VIEW,
    Permission.INJECTABLE_CREATE,
    Permission.INJECTABLE_EDIT,
    Permission.INJECTABLE_TOGGLE_STATUS,
    Permission.DOCUMENT_VIEW,
  ],
  [WorkspaceRole.OPERATOR]: [
    ...COMMON_CONTENT_READ,
    Permission.INJECTABLE_VIEW,
    Permission.DOCUMENT_VIEW,
    Permission.DOCUMENT_CREATE,
    Permission.DOCUMENT_CANCEL,
    Permission.DOCUMENT_DEPRECATE,
    Permission.DOCUMENT_REFRESH,
  ],
  [WorkspaceRole.VIEWER]: [
    ...COMMON_CONTENT_READ,
    Permission.INJECTABLE_VIEW,
    Permission.DOCUMENT_VIEW,
  ],
}

export const TENANT_RULES: Record<TenantRole, Permission[]> = {
  [TenantRole.OWNER]: [
    Permission.TENANT_MANAGE_SETTINGS,
    Permission.TENANT_MANAGE_WORKSPACES,
  ],
  [TenantRole.ADMIN]: [Permission.TENANT_MANAGE_WORKSPACES],
}

export const SYSTEM_RULES: Record<SystemRole, Permission[]> = {
  [SystemRole.SUPERADMIN]: [
    Permission.ADMIN_ACCESS,
    Permission.SYSTEM_TENANTS_VIEW,
    Permission.SYSTEM_TENANTS_MANAGE,
    Permission.SYSTEM_USERS_VIEW,
    Permission.SYSTEM_USERS_MANAGE,
    Permission.SYSTEM_SETTINGS_VIEW,
    Permission.SYSTEM_SETTINGS_MANAGE,
    Permission.SYSTEM_AUDIT_VIEW,
    Permission.SYSTEM_INJECTABLES_VIEW,
    Permission.SYSTEM_INJECTABLES_MANAGE,
    Permission.SYSTEM_API_KEYS_MANAGE,
    Permission.TENANT_CREATE,
    Permission.TENANT_MANAGE_SETTINGS,
    Permission.TENANT_MANAGE_WORKSPACES,
    // Workspace-level permissions (superadmin has full access)
    Permission.WORKSPACE_VIEW,
    Permission.WORKSPACE_UPDATE,
    Permission.WORKSPACE_ARCHIVE,
    Permission.MEMBERS_VIEW,
    Permission.MEMBERS_INVITE,
    Permission.MEMBERS_REMOVE,
    Permission.MEMBERS_UPDATE_ROLE,
    Permission.CONTENT_VIEW,
    Permission.CONTENT_CREATE,
    Permission.CONTENT_EDIT,
    Permission.CONTENT_DELETE,
    Permission.VERSION_VIEW,
    Permission.VERSION_CREATE,
    Permission.VERSION_EDIT_DRAFT,
    Permission.VERSION_DELETE_DRAFT,
    Permission.VERSION_PUBLISH,
    Permission.INJECTABLE_VIEW,
    Permission.INJECTABLE_CREATE,
    Permission.INJECTABLE_EDIT,
    Permission.INJECTABLE_DELETE,
    Permission.INJECTABLE_TOGGLE_STATUS,
    Permission.DOCUMENT_VIEW,
    Permission.DOCUMENT_CREATE,
    Permission.DOCUMENT_CANCEL,
    Permission.DOCUMENT_DEPRECATE,
    Permission.DOCUMENT_REFRESH,
  ],
  [SystemRole.PLATFORM_ADMIN]: [
    Permission.ADMIN_ACCESS,
    Permission.SYSTEM_TENANTS_VIEW,
    Permission.SYSTEM_TENANTS_MANAGE,
    Permission.SYSTEM_AUDIT_VIEW,
    Permission.SYSTEM_INJECTABLES_VIEW,
  ],
}
