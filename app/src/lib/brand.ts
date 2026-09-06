declare const __APP_NAME__: string

/**
 * Product name shown in the UI: the header wordmark, the login and public
 * signing pages, and the document title.
 *
 * Wrappers embed this SPA under their own product identity — `tools-team-signing`
 * ships it as "Tether Signing" — so the name is a build-time constant rather than
 * a hardcoded string. Set `VITE_APP_NAME` when building the app; it defaults to
 * "Doc-Assembly", which is what `tools-doc-assembly` continues to show.
 */
export const APP_NAME: string =
  typeof __APP_NAME__ === 'string' && __APP_NAME__.length > 0
    ? __APP_NAME__
    : 'Doc-Assembly'
