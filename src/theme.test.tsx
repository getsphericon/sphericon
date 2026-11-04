import { Anchor, Table, Text } from '@mantine/core'
import { expect, test } from 'vitest'

import { renderWithProviders } from './test/renderWithProviders.tsx'

const fontSize = (element: Element) => getComputedStyle(element).fontSize

test('a link without a size takes the size of the table cell it stands in', async () => {
  const screen = await renderWithProviders(
    <Table>
      <Table.Tbody>
        <Table.Tr>
          <Table.Td data-testid="cell">
            <Anchor href="#tags">{'Tags'}</Anchor>
          </Table.Td>
        </Table.Tr>
      </Table.Tbody>
    </Table>,
  )

  const link = screen.getByRole('link', { name: 'Tags' }).element()
  expect(fontSize(link)).toBe(fontSize(screen.getByTestId('cell').element()))
})

test('a link keeps the size it is given', async () => {
  const screen = await renderWithProviders(
    <Text size="xl" data-testid="text">
      <Anchor href="#tags" size="xs">
        {'Tags'}
      </Anchor>
    </Text>,
  )

  const link = screen.getByRole('link', { name: 'Tags' }).element()
  expect(fontSize(link)).not.toBe(fontSize(screen.getByTestId('text').element()))
})
