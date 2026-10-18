import type { AnyRoute } from '@tanstack/react-router'

// Where renderWithRouter mounts a page: the real route (so the test router mirrors its
// ancestry and ids, and `Route.useParams()` resolves), its own pattern and the concrete entry
// with its `$param`s filled in.
export function routeMount(route: AnyRoute, params: Record<string, string> = {}) {
  const initialPath = route.fullPath.replaceAll(/\$(\w+)/g, (_: string, name: string) => {
    const value = params[name]
    if (value === undefined) {
      throw new Error(`missing route param ${name}`)
    }
    return encodeURIComponent(value)
  })
  return { route, path: route.fullPath, initialPath }
}
