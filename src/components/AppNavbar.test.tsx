import { expect, test } from 'vitest'

import { Route as activityRoute } from '../routes/workspaces/$slug/activity.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$slug/automations/index.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$slug/contacts/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$slug/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$slug/settings.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$slug/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$slug/transactional-emails/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AppNavbar } from './AppNavbar.tsx'

const SECTIONS = [
  ['Overview', overviewRoute],
  ['Contacts', contactsRoute],
  ['Segments', segmentsRoute],
  ['Broadcasts', broadcastsRoute],
  ['Templates', templatesRoute],
  ['Automations', automationsRoute],
  ['Transactional emails', transactionalEmailsRoute],
  ['Activity', activityRoute],
  ['Settings', settingsRoute],
] as const

test.each(SECTIONS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<AppNavbar slug="acme" />)

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { slug: 'acme' }).initialPath)
})

test('only the section being viewed is current, the overview not on a page under it', async () => {
  const { screen } = await renderWithRouter(
    <AppNavbar slug="acme" />,
    routeMount(contactsRoute, { slug: 'acme' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Contacts', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await expect
    .element(screen.getByRole('link', { name: 'Overview', exact: true }))
    .not.toHaveAttribute('aria-current')
})
