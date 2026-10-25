import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Link,
  RouterProvider,
} from '@tanstack/react-router'
import { expect, test } from 'vitest'
import { page } from 'vitest/browser'

import { Route as loginRoute } from '../routes/login.tsx'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { DOCS_URL } from '../utils/docs.ts'
import { DashboardShell } from './DashboardShell.tsx'

const DESKTOP_WIDTH = 1280
const DESKTOP_HEIGHT = 800
const MOBILE_WIDTH = 500
const MOBILE_HEIGHT = 800
const FIRST_PATH = '/'
const SECOND_PATH = loginRoute.to
const HEADER_SLOT = 'Header slot'
const BANNER_SLOT = 'Banner slot'
const FIRST_PAGE = 'First page'
const SECOND_PAGE = 'Second page'

// A real in-memory router (renderWithRouter stubs navigation) so the shell sees a genuine
// route change: the sidebar links to a second page, and the drawer must close when it opens.
async function mountShell() {
  const rootRoute = createRootRoute()
  const shellRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: 'shell',
    component: () => (
      <DashboardShell
        sidebar={<Link to={SECOND_PATH}>{'Go second'}</Link>}
        headerRight={<span>{HEADER_SLOT}</span>}
        banner={<span>{BANNER_SLOT}</span>}
      />
    ),
  })
  const firstRoute = createRoute({
    getParentRoute: () => shellRoute,
    path: FIRST_PATH,
    component: () => <p>{FIRST_PAGE}</p>,
  })
  const secondRoute = createRoute({
    getParentRoute: () => shellRoute,
    path: SECOND_PATH,
    component: () => <p>{SECOND_PAGE}</p>,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([shellRoute.addChildren([firstRoute, secondRoute])]),
    history: createMemoryHistory({ initialEntries: [FIRST_PATH] }),
  })
  return renderWithProviders(<RouterProvider router={router} />)
}

test('renders the slots around the routed page', async () => {
  const screen = await mountShell()

  await expect.element(screen.getByText(HEADER_SLOT)).toBeInTheDocument()
  await expect.element(screen.getByText(BANNER_SLOT)).toBeInTheDocument()
  await expect.element(screen.getByText(FIRST_PAGE)).toBeInTheDocument()
})

test('the header links to the documentation', async () => {
  // The link is hidden below the `sm` breakpoint, where the header has no room for it.
  await page.viewport(DESKTOP_WIDTH, DESKTOP_HEIGHT)
  const screen = await mountShell()

  await expect
    .element(screen.getByRole('banner').getByRole('link', { name: 'Docs' }))
    .toHaveAttribute('href', DOCS_URL)
})

test('on a narrow viewport navigating closes the opened sidebar', async () => {
  await page.viewport(MOBILE_WIDTH, MOBILE_HEIGHT)
  const screen = await mountShell()

  const burger = screen.getByRole('banner').getByRole('button').first()
  await burger.click()
  await expect.element(screen.getByText('Go second')).toBeInViewport()

  await screen.getByText('Go second').click()

  await expect.element(screen.getByText(SECOND_PAGE)).toBeInTheDocument()
  await expect.element(screen.getByText('Go second')).not.toBeInViewport()
})
