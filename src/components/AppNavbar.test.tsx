import { expect, test } from 'vitest'

import { Route as activityRoute } from '../routes/workspaces/$workspaceId/activity.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as tagsRoute } from '../routes/workspaces/$workspaceId/tags/index.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$workspaceId/transactional-emails/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AppNavbar } from './AppNavbar.tsx'

const SECTIONS = [
  ['Overview', overviewRoute],
  ['Contacts', contactsRoute],
  ['Segments', segmentsRoute],
  ['Tags', tagsRoute],
  ['Broadcasts', broadcastsRoute],
  ['Templates', templatesRoute],
  ['Automations', automationsRoute],
  ['Transactional emails', transactionalEmailsRoute],
  ['Activity', activityRoute],
  ['Settings', settingsRoute],
] as const

test.each(SECTIONS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<AppNavbar workspaceId="AcmeWs01" />)

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { workspaceId: 'AcmeWs01' }).initialPath)
})

test('only the section being viewed is current, the overview not on a page under it', async () => {
  const { screen } = await renderWithRouter(
    <AppNavbar workspaceId="AcmeWs01" />,
    routeMount(contactsRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Contacts', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await expect
    .element(screen.getByRole('link', { name: 'Overview', exact: true }))
    .not.toHaveAttribute('aria-current')
})
