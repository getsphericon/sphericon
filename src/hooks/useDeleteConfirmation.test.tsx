import { expect, test, vi } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useDeleteConfirmation } from './useDeleteConfirmation.tsx'

type Options = Parameters<ReturnType<typeof useDeleteConfirmation>>[0]

const OPEN = 'Open'

function Harness({ options }: { options: Options }) {
  const confirmDelete = useDeleteConfirmation()
  return (
    <button type="button" onClick={() => confirmDelete(options)}>
      {OPEN}
    </button>
  )
}

test('shows the default copy and runs onConfirm when confirmed', async () => {
  const onConfirm = vi.fn<() => void>()
  const screen = await renderWithProviders(<Harness options={{ onConfirm }} />)

  await screen.getByRole('button', { name: OPEN }).click()

  await expect.element(screen.getByText('Are you sure?')).toBeInTheDocument()
  await expect.element(screen.getByText('This action cannot be undone.')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Delete' }).click()
  expect(onConfirm).toHaveBeenCalledTimes(1)
})

test('uses a custom title and description, and cancel does not confirm', async () => {
  const onConfirm = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <Harness options={{ title: 'Remove contact?', description: 'Ada will be gone.', onConfirm }} />,
  )

  await screen.getByRole('button', { name: OPEN }).click()

  await expect.element(screen.getByText('Remove contact?')).toBeInTheDocument()
  await expect.element(screen.getByText('Ada will be gone.')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Cancel' }).click()
  expect(onConfirm).not.toHaveBeenCalled()
})
