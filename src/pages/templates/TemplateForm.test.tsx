import { expect, test } from 'vitest'

import { Route as templatesRoute } from '../../routes/workspaces/$slug/templates/index.tsx'
import { Route as templatesCreateRoute } from '../../routes/workspaces/$slug/templates/new.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { TemplateCreatePage } from './resource.tsx'

test('Cancel links back to the templates list of the current workspace', async () => {
  const { screen } = await renderWithRouter(
    <TemplateCreatePage />,
    routeMount(templatesCreateRoute, { slug: 'test' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(templatesRoute, { slug: 'test' }).initialPath)
})
