import { Tooltip } from '@mantine/core'
import { expect, test } from 'vitest'

import { renderWithProviders } from './renderWithProviders.tsx'

// The two tests run in this order. Playwright's pointer stays where a test left it, so without the
// reset in the global afterEach the second test's tooltip would open under the first one's hover.
const label = ['the', 'label'].join(' ')
const tip = ['the', 'tip'].join(' ')

test('a hover opens the tooltip of the element under the pointer', async () => {
  const screen = await renderWithProviders(
    <Tooltip label={tip}>
      <span>{label}</span>
    </Tooltip>,
  )

  await screen.getByText(label).hover()

  await expect.element(screen.getByText(tip)).toBeInTheDocument()
})

test('the pointer of the previous test is not over what this test renders', async () => {
  const screen = await renderWithProviders(
    <Tooltip label={tip}>
      <span>{label}</span>
    </Tooltip>,
  )

  await expect.element(screen.getByText(label)).toBeInTheDocument()
  // Past the tooltip's open delay, so a pointer left over would have opened it by now.
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 300)
  })
  await expect.element(screen.getByText(tip)).not.toBeInTheDocument()
})
