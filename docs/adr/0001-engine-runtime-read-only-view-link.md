# Mint Read-Only View Links from Engine at runtime

Completion handlers run inside River with no Keycloak token. Wrappers must not import `core/internal` or HTTP-loopback to mint a `VIEW_ONLY` link. We added `Engine.CreateReadOnlyViewLinkByWorkspaceCode` as a post-initialize runtime command that delegates to the existing `ReadOnlyViewUseCase`. HTTP `POST /api/v1/documents/{documentId}/view-link` remains; we did not add a second token type, HMAC route, `/internal/*` PDF endpoint, or `SigningProvider` method.

## Considered Options

- HTTP from the worker — rejected: no Keycloak, wrapping HTTP to self is forbidden
- New token type or HMAC PDF route — rejected: `GET /public/view/{token}/pdf` already serves the completed PDF
- Mint inside `SigningProvider` — rejected: Read-Only View Link issuance is not a signing-provider concern
