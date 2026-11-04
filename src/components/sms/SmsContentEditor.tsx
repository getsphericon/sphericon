import { Group, Menu, Stack, Switch, Text, Textarea } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { CONTACT_TAGS, UNSUBSCRIBE_URL_TAG } from '../../utils/mergeTags.ts'
import { measureSms } from '../../utils/smsParts.ts'

// SmsContent is an SMS's Message content: the text, and whether its links go out as short tracked
// links (ADR 0083).
export interface SmsContent {
  text: string
  linkTracking: boolean
}

interface Props {
  value: SmsContent
  onChange: (next: SmsContent) => void
  // Merge tags of the surface beyond the Contact's (an Automation adds the Event's).
  extraTags?: readonly string[]
}

const NO_TAGS: readonly string[] = []

// SmsContentEditor edits an SMS (ADR 0078): one plain text with a merge-tag picker, and the switch
// that keeps its links as written. Nothing is appended to it, so an opt-out line is the author's to
// write, with `unsubscribe_url` if they want a link. A counter shows the text's characters,
// encoding and segments as it goes out: each link of a tracked text counted as the Workspace's
// short link (`shortLinkLength`, as written until the Workspace loads). The provider's part limit
// is the save's to apply. It holds no state of its own.
export function SmsContentEditor({ value, onChange, extraTags = NO_TAGS }: Props) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const workspacesQuery = useQuery(siteWorkspacesListOptions())
  const linkLength = workspacesQuery.data?.find((w) => w.id === workspaceId)?.shortLinkLength ?? 0
  const field = useRef<HTMLTextAreaElement>(null)
  const tags = [...CONTACT_TAGS, ...extraTags, UNSUBSCRIBE_URL_TAG]
  const size = measureSms(value.text, value.linkTracking ? linkLength : 0)

  const insert = (tag: string) => {
    const at = field.current?.selectionStart ?? value.text.length
    const end = field.current?.selectionEnd ?? at
    onChange({ ...value, text: `${value.text.slice(0, at)}{{ ${tag} }}${value.text.slice(end)}` })
  }

  return (
    <Stack gap="xs">
      <Textarea
        ref={field}
        label={t(($) => $.smsEditor.textLabel)}
        description={t(($) => $.smsEditor.textHint)}
        autosize
        minRows={4}
        maxRows={10}
        value={value.text}
        onChange={(e) => onChange({ ...value, text: e.currentTarget.value })}
      />
      <Group justify="space-between">
        <Menu>
          <Menu.Target>
            <Text component="button" type="button" size="sm" c="blue">
              {t(($) => $.smsEditor.insertMergeTag)}
            </Text>
          </Menu.Target>
          <Menu.Dropdown>
            {tags.map((tag) => (
              <Menu.Item key={tag} onClick={() => insert(tag)}>
                {tag}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
        <Text size="sm" c="dimmed">
          {t(($) => $.smsEditor.counter, {
            characters: t(($) => $.smsEditor.characters, { count: size.characters }),
            encoding: size.encoding,
            segments: t(($) => $.smsEditor.segments, { count: size.parts }),
          })}
        </Text>
      </Group>
      <Switch
        label={t(($) => $.smsEditor.linkTracking)}
        description={t(($) => $.smsEditor.linkTrackingHint)}
        checked={value.linkTracking}
        onChange={(e) => onChange({ ...value, linkTracking: e.currentTarget.checked })}
      />
    </Stack>
  )
}
