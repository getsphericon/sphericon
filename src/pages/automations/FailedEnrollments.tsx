import { Stack, Table, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteEnrollmentsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'

/** The Enrollments an HTTP step failed, with the reason it recorded. Renders nothing while none failed. */
export function FailedEnrollments({ slug, automationId }: { slug: string; automationId: string }) {
  const { t } = useTranslation()
  const failedQuery = useQuery(
    siteEnrollmentsListOptions({
      path: { slug, automationId },
      query: { status: 'failed' },
    }),
  )

  const failed = (failedQuery.data?.items ?? []).flatMap(({ id, contactId, failure }) =>
    failure ? [{ id, contactId, failure }] : [],
  )
  if (failed.length === 0) {
    return null
  }

  return (
    <Stack gap="xs">
      <Title order={5}>{t(($) => $.automations.failedEnrollmentsTitle)}</Title>
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
    </Stack>
  )
}
