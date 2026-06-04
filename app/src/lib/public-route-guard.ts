const PUBLIC_ROUTES = ['/login', '/public']

export function stripBasePath(pathname: string, basePath: string) {
  const normalizedBasePath = basePath.replace(/\/$/, '')

  if (!normalizedBasePath) return pathname
  if (pathname === normalizedBasePath) return '/'
  if (pathname.startsWith(`${normalizedBasePath}/`)) {
    return pathname.slice(normalizedBasePath.length) || '/'
  }

  return pathname
}

export function isPublicPathname(
  pathname: string,
  basePath = import.meta.env.VITE_BASE_PATH || '',
) {
  const appPathname = stripBasePath(pathname, basePath)
  return PUBLIC_ROUTES.some(route => appPathname.startsWith(route))
}
