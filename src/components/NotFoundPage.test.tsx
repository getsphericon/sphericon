import { expect, test } from 'vitest'

import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { NotFoundPage } from './NotFoundPage.tsx'

test('offers a way back to the app', async () => {
  const { screen } = await renderWithRouter(<NotFoundPage />)

  await expect.element(screen.getByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Go to the app' }))
    .toHaveAttribute('href', '/')
})
