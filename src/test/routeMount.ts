import type { AnyRoute } from '@tanstack/react-router'

// Where renderWithRouter mounts a page: the real route (so the test router mirrors its
// ancestry and ids, and `Route.useParams()` resolves), its own pattern and the concrete entry
// with its `$param`s filled in.
export function routeMount(route: AnyRoute, params: Record<string, string> = {}) {
  const filled = route.fullPath.replaceAll(/\$(\w+)/g, (_: string, name: string) => {
    const value = params[name]
    if (value === undefined) {
      throw new Error(`missing route param ${name}`)
    }
    return encodeURIComponent(value)
  })
  // The router drops the trailing slash of an index route from its links; so does the entry.
  const initialPath = filled.length > 1 ? filled.replace(/\/$/, '') : filled
  return { route, path: route.fullPath, initialPath }
}
