import { Alert, Code, Group, List, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { LintFinding } from '../generated/site/types.gen.ts'

// LintWarnings shows the email lint's warnings of a saved message (ADR 0040, ADR 0188): advice that
// never blocks a save, a Schedule or a Publish. Each is shown by what the server says of it, its
// message, with the element and the value it names, whatever its code: a new check shows without a
// change here. The element and the value quote the message or the Brand kit, untrusted text that
// is only ever rendered as text. Nothing renders while there is no warning.
export function LintWarnings({
  warnings,
  title,
}: {
  warnings: LintFinding[] | undefined
  // What the warnings are about, when it is not the message on screen (an Automation's Step).
  title?: string
}) {
  const { t } = useTranslation()
  if (warnings === undefined || warnings.length === 0) {
    return null
  }
  return (
    <Alert
      color="yellow"
      variant="light"
      icon={<IconAlertTriangle size={16} />}
      title={title ?? t(($) => $.lintWarnings.title, { count: warnings.length })}
    >
      <List size="sm" spacing="xs">
        {warnings.map((warning, index) => (
          // A message may hold two findings alike: the list is the server's, in its order.
          <List.Item key={`${warning.code}-${index}`}>
            <Text size="sm">{warning.message}</Text>
            {(warning.element !== undefined || warning.detail !== undefined) && (
              <Group gap="xs" wrap="wrap">
                {warning.element !== undefined && <Code>{warning.element}</Code>}
                {warning.detail !== undefined && <Code>{warning.detail}</Code>}
              </Group>
            )}
          </List.Item>
        ))}
      </List>
    </Alert>
  )
}
