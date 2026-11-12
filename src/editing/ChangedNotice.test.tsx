import { expect, test, vi } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { ChangedNotice } from './ChangedNotice.tsx'

test('loading the saved version over unsaved edits asks first, in the leave dialog words', async () => {
  const onLoad = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <ChangedNotice
      isDirty={() => true}
      busy={false}
      onLoad={onLoad}
      onOverwrite={vi.fn<() => void>()}
    />,
  )

  await screen.getByRole('button', { name: 'Load the saved version' }).click()

  await expect.element(screen.getByText('Your changes are not saved.')).toBeInTheDocument()
  expect(onLoad).not.toHaveBeenCalled()
  await screen.getByRole('dialog').getByRole('button', { name: 'Load the saved version' }).click()
  expect(onLoad).toHaveBeenCalledOnce()
})

test('loading the saved version with nothing unsaved asks nothing', async () => {
  const onLoad = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <ChangedNotice
      isDirty={() => false}
      busy={false}
      onLoad={onLoad}
      onOverwrite={vi.fn<() => void>()}
    />,
  )

  await screen.getByRole('button', { name: 'Load the saved version' }).click()

  expect(onLoad).toHaveBeenCalledOnce()
})

test('saving mine over it is one click', async () => {
  const onOverwrite = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <ChangedNotice
      isDirty={() => true}
      busy={false}
      onLoad={vi.fn<() => void>()}
      onOverwrite={onOverwrite}
    />,
  )

  await screen.getByRole('button', { name: 'Save mine over it' }).click()

  expect(onOverwrite).toHaveBeenCalledOnce()
})
