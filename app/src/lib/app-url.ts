/**
 * Turns a URL returned by the API into one a browser can follow.
 *
 * The API is inconsistent about the server base path, and both directions are
 * wrong in a different way when the app is mounted under one (e.g. `/team` or
 * `/doc-assembly`):
 *
 *   - the read-only view link arrives ALREADY prefixed, because
 *     `buildReadOnlyViewURL` falls back to the server base path when
 *     `server.public_url` is unset. Blindly prefixing it again produced
 *     `/team/team/public/view/{token}`, which matches no route, falls through to
 *     the SPA shell and renders "not found" — with HTTP 200, so nothing warns
 *     the sender that the link they just copied is dead.
 *
 *   - the signed-PDF download link arrives WITHOUT the prefix
 *     (`pre_signing_service.go` builds `/public/sign/{token}/download`), so using
 *     it raw sends the browser to `/public/sign/...` at the origin. The shared
 *     load balancer routes anything outside `/team/*` to a different service
 *     entirely, so the signer gets that service's 404.
 *
 * Prefixing only when the prefix is absent makes both cases correct, and stays
 * correct if the API is later made consistent either way.
 */
export function toAbsoluteAppUrl(url: string): string {
  try {
    // Already absolute (has a scheme) — leave it alone.
    return new URL(url).toString()
  } catch {
    const basePath = (import.meta.env.VITE_BASE_PATH || '').replace(/\/$/, '')
    const path = url.startsWith('/') ? url : `/${url}`

    if (!basePath || hasBasePath(path, basePath)) {
      return `${window.location.origin}${path}`
    }
    return `${window.location.origin}${basePath}${path}`
  }
}

function hasBasePath(path: string, basePath: string): boolean {
  // `/team` itself, or `/team/...` — but NOT `/teamwork`, which merely shares a prefix.
  return path === basePath || path.startsWith(`${basePath}/`)
}
