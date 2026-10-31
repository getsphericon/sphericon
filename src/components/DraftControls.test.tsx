import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { DraftControls } from './DraftControls.tsx'

const NOOP = { mutationFn: async () => Promise.resolve() }

test('a draft above a live version shows the pending badge and Revert; Publish waits for a save', async () => {
  const screen = await renderWithProviders(
    <DraftControls
      workspaceId="TestWs01"
      resource={{ id: '1', status: 'active', live: {}, draft: {} }}
      dirty
      publishMutation={NOOP}
      revertMutation={NOOP}
      messages={{ published: 'Published', reverted: 'Reverted', errorTitle: 'Failed' }}
    />,
  )

  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
})
