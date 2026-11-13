import { expect, test } from 'vitest'

import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { LinkExpiredPage } from './link-expired.tsx'

test('says the link leads nowhere, naming neither sender nor message', async () => {
  const { screen } = await renderWithRouter(<LinkExpiredPage />)

  await expect.element(screen.getByText('This link is no longer available')).toBeInTheDocument()
  await expect
    .element(screen.getByText('It leads nowhere now. The message it came in may be too old.'))
    .toBeInTheDocument()
})
