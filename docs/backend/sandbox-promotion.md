# Sandbox & Promotion Flow

Doc Engine supports a **sandbox environment** for each workspace, allowing users to develop and test templates in isolation before promoting them to production.

## Sandbox Concept

```
Production Workspace (is_sandbox=false)
├── Templates (production-ready)
├── Versions (DRAFT, PUBLISHED, ARCHIVED)
└── Sandbox Workspace (is_sandbox=true, sandbox_of_id=parent)
    ├── Templates (development/testing)
    └── Versions (isolated from production)
```

- Each CLIENT workspace automatically has a sandbox workspace (1:1 relationship)
- Sandbox workspaces are created via database trigger when a CLIENT workspace is created
- **Tags** and **Injectables** are shared between production and sandbox (belong to parent workspace)
- **Templates**, **Versions**, and **Folders** are isolated per environment

## Accessing Sandbox Mode

To operate in sandbox mode, add the `X-Sandbox-Mode: true` header to your requests:

> `X-Sandbox-Mode` is supported only for `CLIENT` workspaces.  
> Requests with `X-Sandbox-Mode: true` on `SYSTEM` workspaces return `400 Bad Request`.

```bash
# List templates in production
curl -X GET /api/v1/content/templates \
  -H "X-Workspace-ID: {workspace-id}" \
  -H "Authorization: Bearer ..."

# List templates in sandbox
curl -X GET /api/v1/content/templates \
  -H "X-Workspace-ID: {workspace-id}" \
  -H "X-Sandbox-Mode: true" \
  -H "Authorization: Bearer ..."
```

## Endpoints with Sandbox Support

| Endpoint | Sandbox Support |
|----------|-----------------|
| `/api/v1/workspace/folders/*` | Yes |
| `/api/v1/content/templates/*` | Yes |
| `/api/v1/content/templates/:id/versions/*` | Yes |
| `/api/v1/documents/*` | Yes |
| `/api/v1/workspace/tags/*` | No (shared) |
| `/api/v1/content/injectables/*` | No (shared) |

## Version Promotion Flow

Once a template version is tested and ready in sandbox, it can be **promoted to production** using the promote endpoint:

```
POST /api/v1/content/templates/:templateId/versions/:versionId/promote
```

### Promotion Modes

| Mode | Description |
|------|-------------|
| `NEW_TEMPLATE` | Creates a new template in production with the promoted version |
| `NEW_VERSION` | Adds the promoted version to an existing production template |

### Request Body

```json
{
  "mode": "NEW_TEMPLATE",
  "targetTemplateId": null,        // Required only for NEW_VERSION
  "targetFolderId": "uuid | null", // Optional, only for NEW_TEMPLATE
  "versionName": "v2.0"            // Optional, default: "Promoted from Sandbox"
}
```

### Promotion Requirements

- Source version **must be PUBLISHED** in sandbox
- **Target workspace must be a production workspace** (not a sandbox) - attempting to promote to a sandbox workspace will result in a `400 Bad Request` error
- Promoted version arrives as **DRAFT** in production (requires review before publishing)
- For `NEW_TEMPLATE`: Template title must be unique in production workspace
- For `NEW_VERSION`: Target template must belong to the production workspace

### What Gets Copied

| Item | NEW_TEMPLATE | NEW_VERSION |
|------|--------------|-------------|
| Content Structure (JSONB) | Yes | Yes |
| Injectables | Yes | Yes |
| Signer Roles | Yes | Yes |
| Tags | Yes | No |

### Example: Promote as New Template

```bash
curl -X POST /api/v1/content/templates/{sandboxTemplateId}/versions/{publishedVersionId}/promote \
  -H "X-Workspace-ID: {prod-workspace-id}" \
  -H "Authorization: Bearer ..." \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "NEW_TEMPLATE",
    "targetFolderId": null,
    "versionName": "Initial Release"
  }'
```

Response:
```json
{
  "template": {
    "id": "new-template-uuid",
    "workspaceId": "prod-workspace-id",
    "title": "Contract Template",
    ...
  },
  "version": {
    "id": "new-version-uuid",
    "templateId": "new-template-uuid",
    "versionNumber": 1,
    "name": "Initial Release",
    "status": "DRAFT",
    ...
  }
}
```

### Example: Promote as New Version

```bash
curl -X POST /api/v1/content/templates/{sandboxTemplateId}/versions/{publishedVersionId}/promote \
  -H "X-Workspace-ID: {prod-workspace-id}" \
  -H "Authorization: Bearer ..." \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "NEW_VERSION",
    "targetTemplateId": "{existingProdTemplateId}",
    "versionName": "v2.0 - New Features"
  }'
```

Response:
```json
{
  "version": {
    "id": "new-version-uuid",
    "templateId": "existing-prod-template-id",
    "versionNumber": 3,
    "name": "v2.0 - New Features",
    "status": "DRAFT",
    ...
  }
}
```

## Typical Workflow

```
1. Create/Edit template in SANDBOX
   └─> Template exists only in sandbox workspace

2. Test and iterate in SANDBOX
   └─> Make changes, preview, validate

3. Publish version in SANDBOX
   └─> Version status: DRAFT → PUBLISHED

4. Promote to PRODUCTION
   └─> POST /promote with mode=NEW_TEMPLATE or NEW_VERSION
   └─> New version created in prod with status: DRAFT

5. Review in PRODUCTION
   └─> Verify content, make final adjustments if needed

6. Publish in PRODUCTION
   └─> POST /:versionId/publish
   └─> Version status: DRAFT → PUBLISHED
   └─> Template now available for document generation
```
