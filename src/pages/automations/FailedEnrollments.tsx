import { Button, Drawer, Table, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteEnrollmentsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'

/**
 * A button with the number of Enrollments an HTTP step failed, opening the reason each recorded.
 * Renders nothing while none failed.
 */
export function FailedEnrollments({
  workspaceId,
  automationId,
}: {
  workspaceId: string
  automationId: string
}) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const failedQuery = useQuery(
    siteEnrollmentsListOptions({
      path: { workspaceId, automationId },
      query: { status: 'failed' },
    }),
  )

  const failed = (failedQuery.data?.items ?? []).flatMap(({ id, contactId, failure }) =>
    failure ? [{ id, contactId, failure }] : [],
  )
  if (failed.length === 0) {
    return null
  }

  const title = t(($) => $.automations.failedEnrollmentsTitle)

  return (
    <>
      <Button color="red" variant="light" onClick={open}>
        {title} ({failed.length})
      </Button>
      <Drawer opened={opened} onClose={close} title={title} position="right" size="xl">
        <Table.ScrollContainer minWidth={420}>
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t(($) => $.automations.failedEnrollmentContact)}</Table.Th>
                <Table.Th>{t(($) => $.automations.failedEnrollmentReason)}</Table.Th>
                <Table.Th>{t(($) => $.automations.failedEnrollmentResponse)}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {failed.map(({ id, contactId, failure }) => (
                <Table.Tr key={id}>
                  <Table.Td>{contactId}</Table.Td>
                  <Table.Td>
                    {t(($) => $.automations.failureKind[failure.kind], {
                      status: Number(failure.status),
                    })}
                  </Table.Td>
                  <Table.Td>
                    <Text size="sm" c="dimmed" lineClamp={2}>
                      {failure.excerpt}
                    </Text>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </Drawer>
    </>
  )
}
