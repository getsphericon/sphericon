import { expect, test, vi } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useConfirmation } from './useConfirmation.tsx'

type Options = Parameters<ReturnType<typeof useConfirmation>>[0]

const OPEN = 'Open'

function Harness({ options }: { options: Options }) {
  const confirmDelete = useConfirmation()
  return (
    <button type="button" onClick={() => confirmDelete(options)}>
      {OPEN}
    </button>
  )
}

test('shows the consequence text and runs onConfirm when confirmed', async () => {
  const onConfirm = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <Harness options={{ description: 'The tag is removed from the catalogue.', onConfirm }} />,
  )

  await screen.getByRole('button', { name: OPEN }).click()

  await expect.element(screen.getByText('Are you sure?')).toBeInTheDocument()
  await expect
    .element(screen.getByText('The tag is removed from the catalogue.'))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Delete' }).click()
  expect(onConfirm).toHaveBeenCalledTimes(1)
})

test('uses a custom title, and cancel does not confirm', async () => {
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

test('typed confirmation keeps the button disabled until the exact name is typed', async () => {
  const onConfirm = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <Harness
      options={{
        description: 'Applications using this key lose access at once.',
        typedName: 'CI key',
        onConfirm,
      }}
    />,
  )

  await screen.getByRole('button', { name: OPEN }).click()

  await expect
    .element(screen.getByText('Applications using this key lose access at once.'))
    .toBeInTheDocument()
  const confirm = screen.getByRole('button', { name: 'Delete' })
  await expect.element(confirm).toBeDisabled()

  const input = screen.getByLabelText('Type CI key to confirm')
  await input.fill('CI ke')
  await expect.element(confirm).toBeDisabled()

  await input.fill('CI key')
  await expect.element(confirm).toBeEnabled()
  await confirm.click()
  expect(onConfirm).toHaveBeenCalledTimes(1)
})
