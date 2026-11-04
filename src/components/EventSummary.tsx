import { Text, Tooltip } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteAutomationsGetOptions,
  siteBroadcastsGetOptions,
  siteCustomFieldsListOptions,
  siteTagsListOptions,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type { JsonObject, SiteEventResource } from '../generated/site/types.gen.ts'
import { zSiteTagsListQuery } from '../generated/site/zod.gen.ts'
import { Route as automationRoute } from '../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as broadcastReportRoute } from '../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as tagsRoute } from '../routes/workspaces/$workspaceId/tags/index.tsx'
import { boundsOf } from '../utils/bounds.ts'
import { shortUrl } from '../utils/shortUrl.ts'
import { EventActionName } from './EventActionName.tsx'
import { AnchorLink } from './RouterLink.tsx'

// TAG_LOOKUP is how many of the Workspace's Tags a row looks its Tag up among, the most a page of
// the contract holds: a Tag beyond them, like a deleted one, is named without a link.
const TAG_LOOKUP = boundsOf(zSiteTagsListQuery.shape.pageSize.unwrap().unwrap()).max

interface Row {
  workspaceId: string
  event: SiteEventResource
}

// EventSummary is an Event's row in a feed (a Contact's events, the Activity feed): a System event
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
    case 'email.sent':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.wasSent)} />
    case 'email.delivered':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.received)} />
    case 'email.delayed':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.delayed)} />
    case 'email.dropped':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.dropped)} />
    case 'email.opened':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.opened)} />
    case 'email.unsubscribed':
      return <EmailEvent {...row} prefix={t(($) => $.eventRows.unsubscribedFrom)} />
    case 'email.complained':
      return (
        <EmailEvent
          {...row}
          prefix={t(($) => $.eventRows.marked)}
          suffix={t(($) => $.eventRows.asSpam)}
        />
      )
    case 'email.bounced': {
      const byKind: Record<string, string> = {
        hard: t(($) => $.eventRows.bouncedHard),
        soft: t(($) => $.eventRows.bouncedSoft),
      }
      const kind = text(event.properties, 'bounceKind') ?? ''
      return <EmailEvent {...row} prefix={byKind[kind] ?? t(($) => $.eventRows.bounced)} />
    }
    case 'email.clicked':
      return <ClickedLink {...row} />
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

// subjectOf is the subject of the email an Event is about, undefined when its message kept none.
function subjectOf(event: SiteEventResource): string | undefined {
  return event.emailSubject === null || event.emailSubject === '' ? undefined : event.emailSubject
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

// EmailSubject is the subject of the email an Event is about, quoted, a link to the Broadcast or
// the Automation that sent it while that still exists. A Broadcast or an Automation that is gone,
// or not loaded yet, leaves the subject plain.
function EmailSubject({ workspaceId, event, subject }: Row & { subject: string }) {
  const { t } = useTranslation()
  const broadcastId = text(event.properties, 'broadcastId')
  const automationId = text(event.properties, 'automationId')
  const broadcastQuery = useQuery({
    ...siteBroadcastsGetOptions({ path: { workspaceId, id: broadcastId ?? '' } }),
    enabled: broadcastId !== undefined,
    retry: false,
  })
  const automationQuery = useQuery({
    ...siteAutomationsGetOptions({ path: { workspaceId, id: automationId ?? '' } }),
    enabled: broadcastId === undefined && automationId !== undefined,
    retry: false,
  })
  const quoted = t(($) => $.eventRows.quoted, { name: subject })
  if (broadcastQuery.data !== undefined) {
    return (
      <AnchorLink
        to={broadcastReportRoute.to}
        params={{ workspaceId, broadcastId: broadcastQuery.data.id }}
      >
        {quoted}
      </AnchorLink>
    )
  }
  if (automationQuery.data !== undefined) {
    return (
      <AnchorLink
        to={automationRoute.to}
        params={{ workspaceId, automationId: automationQuery.data.id }}
      >
        {quoted}
      </AnchorLink>
    )
  }
  return quoted
}

// EmailEvent is the row of an email System event: its verb, the subject the Contact received, and
// what follows it. An Event whose email kept no subject (sent before it was kept, or erased with
// the Contact) shows the action's words alone.
function EmailEvent({
  workspaceId,
  event,
  prefix,
  suffix,
}: Row & { prefix: string; suffix?: string }) {
  const subject = subjectOf(event)
  if (subject === undefined) {
    return <EventActionName action={event.action} />
  }
  return (
    <Text span inherit>
      <Verb action={event.action}>{prefix}</Verb>{' '}
      <EmailSubject workspaceId={workspaceId} event={event} subject={subject} />
      {suffix === undefined ? null : ` ${suffix}`}
    </Text>
  )
}

// ClickedLink is the row of a click: the link without its scheme, a long one shortened with the
// whole one on hover, then the email it was in.
function ClickedLink({ workspaceId, event }: Row) {
  const { t } = useTranslation()
  const subject = subjectOf(event)
  const url = text(event.properties, 'url')
  if (subject === undefined || url === undefined) {
    return <EventActionName action={event.action} />
  }
  return (
    <Text span inherit>
      <Tooltip label={event.action}>
        <Text span inherit>
          {t(($) => $.eventRows.clicked)}
        </Text>
      </Tooltip>{' '}
      <Tooltip label={url}>
        <Text span inherit>
          {shortUrl(url)}
        </Text>
      </Tooltip>{' '}
      {t(($) => $.eventRows.in)}{' '}
      <EmailSubject workspaceId={workspaceId} event={event} subject={subject} />
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
