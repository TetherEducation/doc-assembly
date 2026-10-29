/**
 * API Response Types
 * Based on ../core/docs/swagger.yaml definitions
 * Use the MCP `doc-engine-api` to get the latest definitions
 */

// ============================================
// Common Types
// ============================================

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    page: number
    perPage: number
    total: number
    totalPages: number
  }
}

export interface ApiError {
  code: string
  error: string
  message: string
}

// ============================================
// Injectable Types
// ============================================

export type InjectableDataType =
  | 'TEXT'
  | 'NUMBER'
  | 'DATE'
  | 'CURRENCY'
  | 'BOOLEAN'
  | 'IMAGE'
  | 'TABLE'

export type InjectableSourceType = 'INTERNAL' | 'EXTERNAL'

export interface Injectable {
  id: string
  workspaceId?: string
  key: string
  /** i18n map; workspace fill-ins arrive as {"_": text} (injectable_mapper.go) */
  label: Record<string, string>
  description?: Record<string, string>
  dataType: InjectableDataType
  sourceType: InjectableSourceType
  metadata?: Record<string, unknown>
  isGlobal: boolean
  createdAt: string
  updatedAt?: string
}

export interface TemplateVersionInjectable {
  id: string
  templateVersionId: string
  definition: Injectable
  isRequired: boolean
  defaultValue?: string
  createdAt: string
}

// ============================================
// Signer Role Types
// ============================================

export interface SignerRole {
  id: string
  templateVersionId: string
  roleName: string
  signerOrder: number
  anchorString: string
  createdAt: string
  updatedAt?: string
}

// ============================================
// Tag Types
// ============================================

export interface Tag {
  id: string
  workspaceId: string
  name: string
  color: string
  createdAt: string
  updatedAt?: string
}

export interface TagWithCount extends Tag {
  templateCount: number
}

// ============================================
// Folder Types
// ============================================

export interface Folder {
  id: string
  workspaceId: string
  parentId?: string
  name: string
  childFolderCount: number
  templateCount: number
  createdAt: string
  updatedAt?: string
}

export interface FolderTree extends Folder {
  children: FolderTree[]
}

// ============================================
// Template Version Types
// ============================================

export type VersionStatus = 'DRAFT' | 'SCHEDULED' | 'PUBLISHED' | 'ARCHIVED'

export interface TemplateVersionDetail {
  id: string
  templateId: string
  versionNumber: number
  name: string
  description?: string
  status: VersionStatus
  contentStructure: number[]
  injectables: TemplateVersionInjectable[]
  signerRoles: SignerRole[]
  publishedAt?: string
  publishedBy?: string
  archivedAt?: string
  archivedBy?: string
  scheduledPublishAt?: string
  scheduledArchiveAt?: string
  createdAt: string
  createdBy?: string
  updatedAt?: string
}

export interface TemplateVersionListItem {
  id: string
  templateId: string
  versionNumber: number
  name: string
  status: VersionStatus
  createdAt: string
  updatedAt?: string
}

// ============================================
// Template Types
// ============================================

export interface Template {
  id: string
  workspaceId: string
  folderId?: string
  title: string
  isPublicLibrary: boolean
  documentTypeId?: string | null
  documentTypeName?: Record<string, string> | null
  process?: string
  processType?: string
  createdAt: string
  updatedAt?: string
}

export interface TemplateListItem extends Template {
  tags: Tag[]
  documentTypeCode?: string
  hasPublishedVersion: boolean
  publishedVersionNumber?: number
  versionCount: number
  scheduledVersionCount: number
}

export interface TemplateWithVersions extends Template {
  versions: TemplateVersionListItem[]
}

// Template with all versions (from /all-versions endpoint)
export interface TemplateWithAllVersionsResponse {
  id: string
  workspaceId: string
  title: string
  folderId?: string
  folder?: Folder
  isPublicLibrary: boolean
  tags: Tag[]
  versions: TemplateVersionSummaryResponse[]
  documentTypeId?: string | null
  documentTypeName?: Record<string, string> | null
  process?: string
  processType?: string
  createdAt: string
  updatedAt?: string
}

// Version summary (includes injectables and signer roles)
export interface TemplateVersionSummaryResponse {
  id: string
  templateId: string
  versionNumber: number
  name: string
  description?: string
  status: VersionStatus
  injectables: TemplateVersionInjectable[]
  signerRoles: SignerRole[]
  createdAt: string
  createdBy?: string
  publishedAt?: string
  publishedBy?: string
  archivedAt?: string
  archivedBy?: string
  scheduledPublishAt?: string
  scheduledArchiveAt?: string
  updatedAt?: string
}

// List versions response
export interface ListTemplateVersionsResponse {
  items: TemplateVersionSummaryResponse[]
  total: number
}

// Template version response (for create/update)
export interface TemplateVersionResponse {
  id: string
  templateId: string
  versionNumber: number
  name: string
  description?: string
  status: VersionStatus
  createdAt: string
  createdBy?: string
  publishedAt?: string
  publishedBy?: string
  archivedAt?: string
  archivedBy?: string
  scheduledPublishAt?: string
  scheduledArchiveAt?: string
  updatedAt?: string
}

export interface TemplateCreateResponse {
  template: Template
  initialVersion: TemplateVersionListItem
}

// ============================================
// Member Types
// ============================================

export type MembershipStatus = 'PENDING' | 'ACTIVE'
export type UserStatus = 'INVITED' | 'ACTIVE' | 'SUSPENDED'

export interface MemberUser {
  id: string
  email: string
  fullName: string
  status: UserStatus
}

export interface WorkspaceMember {
  id: string
  workspaceId: string
  user: MemberUser
  role: string
  membershipStatus: MembershipStatus
  joinedAt?: string
  createdAt: string
}

export interface TenantMember {
  id: string
  tenantId: string
  user: MemberUser
  role: string
  membershipStatus: MembershipStatus
  createdAt: string
}

export type SystemUserRole = 'SUPERADMIN' | 'PLATFORM_ADMIN'

export interface SystemUser {
  id: string
  userId: string
  role: SystemUserRole
  grantedBy?: string
  createdAt: string
  user: MemberUser
}

// ============================================
// Request Types
// ============================================

export interface CreateTemplateRequest {
  title: string
  folderId?: string
  isPublicLibrary?: boolean
}

export interface UpdateTemplateRequest {
  title?: string
  folderId?: string
  isPublicLibrary?: boolean
  documentTypeId?: string | null
}

export interface CreateVersionRequest {
  name: string
  description?: string
}

export interface CreateVersionFromExistingRequest {
  sourceVersionId: string
  name: string
  description?: string
}

export interface UpdateVersionRequest {
  name?: string
  description?: string
  contentStructure?: Record<string, unknown>
}

export interface CreateFolderRequest {
  name: string
  parentId?: string
}

export interface UpdateFolderRequest {
  name?: string
}

export interface MoveFolderRequest {
  parentId: string | null
}

export interface CreateTagRequest {
  name: string
  color?: string
}

export interface UpdateTagRequest {
  name?: string
  color?: string
}

export interface AddInjectableRequest {
  injectableId: string
  isRequired?: boolean
  defaultValue?: string
}

export interface SchedulePublishRequest {
  scheduledAt: string
}

export interface PreviewRequest {
  values: Record<string, unknown>
}
