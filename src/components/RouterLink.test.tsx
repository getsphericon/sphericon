import { expect, test } from 'vitest'

import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { ButtonLink } from './RouterLink.tsx'

test('ButtonLink renders a real anchor to the typed route', async () => {
  const { screen } = await renderWithRouter(
    <ButtonLink to="/workspaces/$workspaceId" params={{ workspaceId: 'AcmeWs01' }}>
      Go
    </ButtonLink>,
  )

  await expect
    .element(screen.getByRole('link', { name: 'Go' }))
    .toHaveAttribute('href', '/workspaces/AcmeWs01')
})
