import {
  type AnyRoute,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { type MockInstance, vi } from 'vitest'

import { renderWithProviders } from './renderWithProviders.tsx'

type RenderWithRouterOptions = {
  // Memory-history entry the router starts on. The default carries a `slug`
  // param so components calling useParams({ strict: false }) see one, matching
  // the app's workspace-scoped routes.
  initialPath?: string
  // Route pattern the component is mounted under. Must line up with initialPath.
  path?: string
  // The real route the component is the page of (from routeMount): the test router mirrors
  // its ancestry, so route ids, `route.useParams()` and `useParams({ from })` resolve as in
  // the app. Without it the component is mounted flat under `path`.
  route?: AnyRoute
}

// Bare routes with the same ids as `route` and each of its ancestors, rendering `ui` at the
// leaf. Guards and layouts are the app's business, not the page's.
function mirroredTree(route: AnyRoute, ui: ReactNode) {
  const chain: AnyRoute[] = []
  for (let r = route; !r.isRoot; r = r.options.getParentRoute()) chain.unshift(r)

  const root = createRootRoute()
  const routes: AnyRoute[] = []
  for (const [index, { options }] of chain.entries()) {
    const parent: AnyRoute = routes[index - 1] ?? root
    const component = index === chain.length - 1 ? () => <>{ui}</> : Outlet
    const getParentRoute = () => parent
    routes.push(
      'path' in options
        ? createRoute({ getParentRoute, path: options.path, component })
        : createRoute({ getParentRoute, id: options.id, component }),
    )
  }
  const nested = routes.reduceRight((child, r) => r.addChildren([child]))
  return root.addChildren([nested])
}

function flatTree(path: string, ui: ReactNode) {
  const root = createRootRoute()
  return root.addChildren([
    createRoute({ getParentRoute: () => root, path, component: () => <>{ui}</> }),
  ])
}

// renderWithRouter mounts `ui` inside a minimal in-memory TanStack router so
// components that call useNavigate/useParams render without the real route tree.
// Navigation is not actually performed — `router.navigate` is replaced with a
// spy the test can assert against (e.g. expect(navigate).toHaveBeenCalledWith).
export async function renderWithRouter(
  ui: ReactNode,
  {
    initialPath = '/workspaces/test',
    path = '/workspaces/$slug',
    route,
  }: RenderWithRouterOptions = {},
) {
  const router = createRouter({
    routeTree: route ? mirroredTree(route, ui) : flatTree(path, ui),
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })

  // Typed as MockInstance so the inferred return type stays nameable (no leaked
  // @vitest/spy internals). navigate is stubbed so tests can assert on it.
  const navigate: MockInstance<typeof router.navigate> = vi
    .spyOn(router, 'navigate')
    .mockResolvedValue(undefined)

  const screen = await renderWithProviders(<RouterProvider router={router} />)
  return { screen, navigate }
}
