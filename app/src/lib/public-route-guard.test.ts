import { describe, expect, it } from 'vitest'
import { isPublicPathname, stripBasePath } from './public-route-guard'

describe('public route guard', () => {
  it('treats public read-only routes under the configured base path as public', () => {
    expect(
      isPublicPathname('/doc-assembly/public/view/view-token', '/doc-assembly'),
    ).toBe(true)
  })

  it('does not classify private routes under the configured base path as public', () => {
    expect(
      isPublicPathname('/doc-assembly/workspace/ws-1/signing', '/doc-assembly'),
    ).toBe(false)
  })

  it('normalizes the configured base path before comparing app routes', () => {
    expect(stripBasePath('/doc-assembly/login', '/doc-assembly/')).toBe('/login')
  })
})
