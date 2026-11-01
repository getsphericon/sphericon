import { Text, Tooltip } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteAutomationsGetOptions,
  siteCustomFieldsListOptions,
  siteTagsListOptions,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type { JsonObject, SiteEventResource } from '../generated/site/types.gen.ts'
import { zSiteTagsListQuery } from '../generated/site/zod.gen.ts'
import { Route as automationRoute } from '../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as tagsRoute } from '../routes/workspaces/$workspaceId/tags/index.tsx'
import { boundsOf } from '../utils/bounds.ts'
import { EventActionName } from './EventActionName.tsx'
import { AnchorLink } from './RouterLink.tsx'

// TAG_LOOKUP is how many of the Workspace's Tags a row looks its Tag up among, the most a page of
// the contract holds: a Tag beyond them, like a deleted one, is named without a link.
const TAG_LOOKUP = boundsOf(zSiteTagsListQuery.shape.pageSize.unwrap().unwrap()).max

interface Row {
  workspaceId: string
  event: SiteEventResource
}

// EventSummary is an Event's row in a feed (a Contact's events, Recent activity): a System event
// about a Contact names what it is about, the Tag, the Custom field or the Automation, as a link
// while that still exists; a page view names the page. Any other Event shows its action by its
// words (EventActionName).
export function EventSummary({ workspaceId, event }: Row) {
  const { t } = useTranslation()
  const row = { workspaceId, event }
  switch (event.action) {
    case 'contact.tagged':
      return <TagChange {...row} verb={t(($) => $.eventRows.tagged)} />
    case 'contact.untagged':
      return <TagChange {...row} verb={t(($) => $.eventRows.untagged)} />
    case 'contact.field_changed':
      return <FieldChange {...row} />
    case 'automation.enrolled':
      return <EnrollmentChange {...row} verb={t(($) => $.eventRows.entered)} />
    case 'automation.completed':
      return <EnrollmentChange {...row} verb={t(($) => $.eventRows.completed)} />
    case 'automation.goal_reached':
      return <EnrollmentChange {...row} verb={t(($) => $.eventRows.reachedGoalOf)} />
    case 'automation.exited':
      return <EnrollmentChange {...row} verb={t(($) => $.eventRows.left)} />
    case 'page.view':
      return <PageView event={event} />
    default:
      return <EventActionName action={event.action} />
  }
}

// text is a property of the Event as text, undefined when it is absent or empty.
function text(properties: JsonObject | null | undefined, key: string): string | undefined {
  const value = properties?.[key]
  if (value === undefined || value === null || value === '') {
    return undefined
  }
  return typeof value === 'string' ? value : JSON.stringify(value)
}

// Verb is the row's verb, its action's code on hover as EventActionName's.
function Verb({ action, children }: { action: string; children: string }) {
  return (
    <Tooltip label={action}>
      <Text span inherit>
        {children}
      </Text>
    </Tooltip>
  )
}

function TagChange({ workspaceId, event, verb }: Row & { verb: string }) {
  const { t } = useTranslation()
  const name = text(event.properties, 'tag')
  const tagsQuery = useQuery({
    ...siteTagsListOptions({ path: { workspaceId }, query: { pageSize: TAG_LOOKUP } }),
    enabled: name !== undefined,
  })
  if (name === undefined) {
    return <EventActionName action={event.action} />
  }
  const quoted = t(($) => $.eventRows.quoted, { name })
  const exists = tagsQuery.data?.items.some((tag) => tag.name === name) ?? false
  return (
    <Text span inherit>
      <Verb action={event.action}>{verb}</Verb>{' '}
      {exists ? (
        <AnchorLink to={tagsRoute.to} params={{ workspaceId }}>
          {quoted}
        </AnchorLink>
      ) : (
        quoted
      )}
    </Text>
  )
}

function FieldChange({ workspaceId, event }: Row) {
  const { t } = useTranslation()
  const key = text(event.properties, 'key')
  const fieldsQuery = useQuery({
    ...siteCustomFieldsListOptions({ path: { workspaceId } }),
    enabled: key !== undefined,
  })
  if (key === undefined) {
    return <EventActionName action={event.action} />
  }
  const field = fieldsQuery.data?.items.find((f) => f.key === key)?.name ?? key
  const value = text(event.properties, 'value')
  return (
    <Verb action={event.action}>
      {value === undefined
        ? t(($) => $.eventRows.fieldCleared, { field })
        : t(($) => $.eventRows.fieldSet, { field, value })}
    </Verb>
  )
}

function EnrollmentChange({ workspaceId, event, verb }: Row & { verb: string }) {
  const { t } = useTranslation()
  const id = text(event.properties, 'automationId')
  const automationQuery = useQuery({
    ...siteAutomationsGetOptions({ path: { workspaceId, id: id ?? '' } }),
    enabled: id !== undefined,
    retry: false,
  })
  const automation = automationQuery.data
  if (automation === undefined) {
    // An Automation purged since, or not loaded yet, is named by the action's words alone.
    return <EventActionName action={event.action} />
  }
  return (
    <Text span inherit>
      <Verb action={event.action}>{verb}</Verb>{' '}
      <AnchorLink to={automationRoute.to} params={{ workspaceId, automationId: automation.id }}>
        {t(($) => $.eventRows.quoted, { name: automation.name })}
      </AnchorLink>
    </Text>
  )
}

function PageView({ event }: { event: SiteEventResource }) {
  const { t } = useTranslation()
  const page = text(event.properties, 'title') ?? text(event.properties, 'path')
  if (page === undefined) {
    return <EventActionName action={event.action} />
  }
  return (
    <Tooltip label={text(event.properties, 'url') ?? event.action}>
      <Text span inherit>
        {t(($) => $.eventRows.visited, { name: page })}
      </Text>
    </Tooltip>
  )
}
