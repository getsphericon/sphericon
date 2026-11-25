import { Button, Checkbox, Group, Stack, Text } from '@mantine/core'
import { QueryBuilderMantine } from '@react-querybuilder/mantine'
import { useMutation, useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QueryBuilder } from 'react-querybuilder'
import type { Field, Operator } from 'react-querybuilder'

import {
  siteAutomationsListOptions,
  siteBroadcastsListOptions,
  siteCustomFieldsListOptions,
  siteEventsActionsOptions,
  siteEventsPropertyKeysOptions,
  siteSegmentsPreviewMutation,
  siteTagsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  ContactAttribute,
  EngagementTrackingChannel,
  EngagementTrackingState,
  EventCountOperator,
  HasOperator,
  MessageSender,
  SystemEventAction,
} from '../../generated/site/types.gen.ts'
import { useEventActionLabel } from '../../hooks/useEventActionLabel.ts'
import { useTimeZone } from '../../hooks/useFormat.ts'
import {
  customFieldId,
  eventFieldPrefix,
  eventPresets,
  sentByFieldId,
  sentByOperator,
  subjectFields,
  subjectless,
  toCondition,
  toDraft,
  toQuery,
  trackingFieldId,
} from './condition.ts'
import type { AnyCondition, ConditionDraft, ConditionPlace } from './condition.ts'
import { ConditionValueEditor } from './ConditionValueEditor.tsx'
import {
  attributeOperators,
  birthdayOperators,
  haveOperators,
  inputTypeOf,
  inputTypeOfField,
  operatorsOf,
  propertyOperators,
  tagOperators,
  trackingOperators,
} from './fieldOperators.ts'
import type {
  EventFilterEditorProps,
  PropertyValues,
  ValueEditorContext,
} from './valueEditorContext.ts'

// Event conditions count the action (per-field overrides the global operator set): the presets
// "performed" and "not performed", or at least / at most a number of times. EventCountEditor
// edits the count and the window in days (blank = ever).
function eventOperators(t: TFunction): Operator[] {
  return [
    { name: eventPresets.performed, label: t(($) => $.conditions.eventOperators.performed) },
    { name: eventPresets.notPerformed, label: t(($) => $.conditions.eventOperators.notPerformed) },
    { name: EventCountOperator.AT_LEAST, label: t(($) => $.conditions.eventOperators.atLeast) },
    { name: EventCountOperator.AT_MOST, label: t(($) => $.conditions.eventOperators.atMost) },
  ]
}

// The Contact's leaves: its text attributes, its birthday, its Tags, whether it can be reached on
// Telegram or by SMS, and its Engagement tracking consent on each Channel.
function contactFields(t: TFunction): Field[] {
  return [
    ...Object.values(ContactAttribute).map((attribute) => ({
      name: attribute,
      label: t(($) => $.conditions.attributes[attribute]),
      operators: attributeOperators(t),
    })),
    {
      name: subjectFields.birthday,
      label: t(($) => $.conditions.fields.birthday),
      operators: birthdayOperators(t),
    },
    // Whether the Contact has the Tag typed as the value.
    {
      name: subjectFields.tag,
      label: t(($) => $.conditions.fields.tag),
      operators: tagOperators(t),
    },
    // Whether a Telegram chat is bound to the Contact or its Visitors: no value to type.
    {
      name: subjectFields.telegram,
      label: t(($) => $.conditions.fields.telegram),
      operators: haveOperators(t),
      defaultOperator: HasOperator.HAS,
    },
    // Whether the Contact's phone can be texted (it has an SMS Confirmation and no SMS opt-out): no
    // value to type.
    {
      name: subjectFields.sms,
      label: t(($) => $.conditions.fields.sms),
      operators: haveOperators(t),
      defaultOperator: HasOperator.HAS,
    },
    // The consent recorded for the Channel's Destination (granted or denied), picked from a list,
    // or whether one is recorded at all.
    ...Object.values(EngagementTrackingChannel).map((channel) => ({
      name: trackingFieldId(channel),
      label: t(($) => $.conditions.trackingFields[channel]),
      operators: trackingOperators(t),
      valueEditorType: 'select' as const,
      values: Object.values(EngagementTrackingState).map((state) => ({
        name: state,
        label: t(($) => $.conditions.trackingStates[state]),
      })),
    })),
  ]
}

// A property of the Event, its key typed beside it.
function propertyField(t: TFunction): Field {
  return {
    name: subjectFields.property,
    label: t(($) => $.conditions.fields.property),
    operators: propertyOperators(t),
  }
}

// The fields each place offers, given the Contact's (its attributes, Custom fields and history): a
// Segment's tests the Contact, a Rule's also a property of the Event that fired it, a Branch path's
// a property of the Event its Enrollment began with, and an Event filter only the properties of the
// Event it is about.
const placeFields: Record<ConditionPlace, (t: TFunction, contact: Field[]) => Field[]> = {
  segment: (_t, contact) => contact,
  rule: (t, contact) => [...contact, propertyField(t)],
  branch: (t, contact) => [...contact, propertyField(t)],
  eventFilter: (t) => [propertyField(t)],
}

// What a place reads of the Workspace's catalogue to offer the Contact's fields: an Event filter
// offers none.
function testsContact(place: ConditionPlace): boolean {
  return place !== 'eventFilter'
}

interface ConditionBuilderProps {
  workspaceId: string
  // The Condition the builder starts from. The builder keeps its own state from then on (an
  // operator the user picked stays picked, a count can be blank while it is typed), so a later
  // change of this value is not read: a form that loads another Condition remounts the builder.
  initialValue: AnyCondition
  // Every edit, as a draft: toCondition is undefined while an event count, a number, a day or a
  // property's key is blank.
  onChange: (draft: ConditionDraft) => void
  // Whose Condition it is (ADR 0181): the builder offers what the place admits. A Rule has an
  // Event, whose properties it tests beside the Contact, an Event filter only the Event's
  // properties; only a Segment's Condition is previewed.
  place: ConditionPlace
  // The values offered for a property's text test, by the property's key: the names of the
  // catalogue a property holds (an Event filter's `tag` of contact.tagged). A suggestion, not a
  // limit: the server checks the name.
  propertyValues?: PropertyValues
  // The keys offered for a property test: those seen on recent Events of the action an Event filter
  // is about. A suggestion, not a limit.
  propertyKeys?: readonly string[]
  // The fields an Event filter adds to its property tests: what sent a message's System event.
  filterFields?: Field[]
}

// ConditionBuilder is the visual Condition editor (react-querybuilder + the official
// Mantine compat package — no hand-rolled condition rows, no custom CSS). It edits the
// API's typed rule group and offers a live preview of how many active contacts match.
export function ConditionBuilder({
  workspaceId,
  initialValue,
  onChange,
  place,
  propertyValues,
  propertyKeys,
  filterFields,
}: ConditionBuilderProps) {
  const { t } = useTranslation()
  const labelOf = useEventActionLabel()
  // The days of before and after are days in the User time zone (ADR 0068).
  const timeZone = useTimeZone()
  const [initialQuery] = useState(() => toQuery(initialValue, timeZone))
  const [draft, setDraft] = useState<ConditionDraft>(initialValue)
  const condition = toCondition(draft, 'segment')
  const preview = useMutation(siteSegmentsPreviewMutation())

  // The workspace's distinct event actions become behavioral fields ("event:<action>"), each an
  // event leaf: a count of the action in the events log.
  const actionsQuery = useQuery({
    ...siteEventsActionsOptions({ path: { workspaceId } }),
    enabled: testsContact(place),
  })
  // The typed Custom field catalogue (ADR 0006) becomes targetable fields alongside the core
  // fields — one governed attribute list. Each offers the operators the server lists for its type,
  // per typed test; the operator picks the value input (inputTypeOf), a number field's own is a
  // number.
  const customFieldsQuery = useQuery({
    ...siteCustomFieldsListOptions({ path: { workspaceId } }),
    enabled: testsContact(place),
  })
  const fields = useMemo<Field[]>(() => {
    const customFields = (customFieldsQuery.data?.items ?? []).map<Field>((f) => ({
      name: customFieldId(f.key),
      label: f.name,
      operators: operatorsOf(t, f),
      inputType: inputTypeOfField(f),
    }))
    // A System event reads as its name ("Opened an email"); a customer's action as it was tracked.
    const eventFields = (actionsQuery.data?.actions ?? []).map<Field>((action) => ({
      name: `${eventFieldPrefix}${action}`,
      label:
        labelOf(action) === action ? t(($) => $.segments.didEvent, { action }) : labelOf(action),
      operators: eventOperators(t),
      defaultOperator: eventPresets.performed,
    }))
    return [
      ...placeFields[place](t, [...contactFields(t), ...customFields, ...eventFields]),
      ...(filterFields ?? []),
    ]
  }, [actionsQuery.data, customFieldsQuery.data, labelOf, t, place, filterFields])
  // What the value editors read: the values and keys a property test is offered, and, where an
  // event test is edited, the Workspace, the editor of its filter and how far back Events are kept.
  const context: ValueEditorContext = {
    ...(propertyValues === undefined ? {} : { propertyValues }),
    ...(propertyKeys === undefined ? {} : { propertyKeys }),
    ...(testsContact(place)
      ? {
          workspaceId,
          eventFilterEditor: EventFilterBuilder,
          ...(actionsQuery.data
            ? {
                eventsKeptDays: actionsQuery.data.eventsKeptDays,
                keptActions: actionsQuery.data.keptActions,
              }
            : {}),
        }
      : {}),
  }

  return (
    <Stack gap="xs">
      <QueryBuilderMantine>
        <QueryBuilder
          fields={fields}
          operators={attributeOperators(t)}
          getInputType={(_field, operator, { fieldData }) => inputTypeOf(operator, fieldData)}
          defaultQuery={initialQuery}
          enableMountQueryChange={false}
          controlElements={{ valueEditor: ConditionValueEditor }}
          context={context}
          onQueryChange={(q) => {
            const next = toDraft(q, timeZone)
            setDraft(next)
            onChange(next)
          }}
        />
      </QueryBuilderMantine>
      {place === 'segment' ? (
        <Group>
          <Button
            variant="light"
            size="compact-sm"
            loading={preview.isPending}
            disabled={condition === undefined}
            onClick={() => {
              if (condition) {
                preview.mutate({ path: { workspaceId }, body: { condition } })
              }
            }}
          >
            {t(($) => $.segments.previewButton)}
          </Button>
          {preview.data ? (
            <Text size="sm">
              {t(($) => $.segments.previewCount, { count: preview.data.count })}
            </Text>
          ) : null}
          {/* A count of opens or clicks is unknown for a Contact who is not tracked (ADR 0123): it
              is left out either way, and the author sees how many. */}
          {preview.data && preview.data.unknownEngagement > 0 ? (
            <Text size="sm" c="dimmed">
              {t(($) => $.segments.previewUnknownEngagement, {
                count: preview.data.unknownEngagement,
              })}
            </Text>
          ) : null}
          {/* An event test with no window, or a longer one, counts only the Events kept (ADR
              0153): the preview says how far back they reach. */}
          {preview.data?.eventsKeptDays === undefined ? null : (
            <Text size="sm" c="dimmed">
              {t(($) => $.segments.eventsKept, { count: preview.data.eventsKeptDays })}
            </Text>
          )}
        </Group>
      ) : null}
    </Stack>
  )
}

// How many Tags, Broadcasts and Automations an Event filter offers: the first page of each list,
// as a suggestion.
const SUGGESTIONS = 100

// The actions whose `tag` names a Tag of the catalogue.
const TAG_ACTIONS = new Set<string>([
  SystemEventAction.CONTACT_TAGGED,
  SystemEventAction.CONTACT_UNTAGGED,
])

// A Broadcast or an Automation as a sent-by test offers it: its id and its name, marked when it is
// archived.
interface Sender {
  id: string
  name: string
  archivedAt?: string | null
}

// senderField is the field of the test that a sender of the kind sent the message, offering the
// senders given.
function senderField(t: TFunction, sender: MessageSender, senders: Sender[]): Field {
  const labels: Record<MessageSender, string> = {
    [MessageSender.BROADCAST]: t(($) => $.conditions.sentBy.broadcast),
    [MessageSender.AUTOMATION]: t(($) => $.conditions.sentBy.automation),
  }
  return {
    name: sentByFieldId(sender),
    label: labels[sender],
    operators: [{ name: sentByOperator, label: t(($) => $.conditions.sentByOperator) }],
    valueEditorType: 'select',
    values: senders.map((row) => ({
      name: row.id,
      label: row.archivedAt ? t(($) => $.conditions.archivedSender, { name: row.name }) : row.name,
    })),
  }
}

// namesSender tells a filter that holds a sent-by test, at any depth.
function namesSender(filter: AnyCondition | ConditionDraft): boolean {
  return filter.entries.some(
    (node: AnyCondition['entries'][number] | ConditionDraft['entries'][number]) =>
      node.kind === 'group' ? namesSender(node) : node.kind === 'sentBy',
  )
}

// EventFilterBuilder is the builder of the filter of the Events of an action (ADR 0133, ADR 0153):
// an Event matcher's, and an event test's in a Condition. It offers the keys seen on recent Events
// of the action, the Workspace's Tags for the `tag` of contact.tagged and contact.untagged and its
// Custom fields for the `key` of contact.field_changed, which the server checks, and, on a message's
// System event, the Broadcasts and Automations that may have sent it: the kept ones, and the
// archived ones on request, marked as archived. It hands back the filter as it holds it, its
// property tests naming no subject.
export function EventFilterBuilder({
  workspaceId,
  action,
  initialValue,
  onChange,
}: EventFilterEditorProps) {
  const { t } = useTranslation()
  const [archived, setArchived] = useState(() => namesSender(initialValue))
  const actions = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  const keys = useQuery(siteEventsPropertyKeysOptions({ path: { workspaceId }, query: { action } }))
  const message = actions.data?.messageActions.includes(action) ?? false
  const tags = useQuery({
    ...siteTagsListOptions({ path: { workspaceId }, query: { pageSize: SUGGESTIONS } }),
    enabled: TAG_ACTIONS.has(action),
  })
  const fields = useQuery({
    ...siteCustomFieldsListOptions({ path: { workspaceId } }),
    enabled: action === SystemEventAction.CONTACT_FIELD_CHANGED,
  })
  const page = (withArchived: boolean) => ({
    path: { workspaceId },
    query: { pageSize: SUGGESTIONS, archived: withArchived },
  })
  const broadcasts = useQuery({ ...siteBroadcastsListOptions(page(false)), enabled: message })
  const automations = useQuery({ ...siteAutomationsListOptions(page(false)), enabled: message })
  const oldBroadcasts = useQuery({
    ...siteBroadcastsListOptions(page(true)),
    enabled: message && archived,
  })
  const oldAutomations = useQuery({
    ...siteAutomationsListOptions(page(true)),
    enabled: message && archived,
  })
  const offered: PropertyValues = {}
  if (TAG_ACTIONS.has(action)) {
    offered.tag = (tags.data?.items ?? []).map((tag) => tag.name)
  }
  if (action === SystemEventAction.CONTACT_FIELD_CHANGED) {
    offered.key = (fields.data?.items ?? []).map((field) => field.key)
  }
  const senderFields = useMemo<Field[]>(() => {
    if (!message) {
      return []
    }
    const old = (rows: Sender[] = []) => (archived ? rows : [])
    return [
      senderField(t, MessageSender.BROADCAST, [
        ...(broadcasts.data?.items ?? []),
        ...old(oldBroadcasts.data?.items),
      ]),
      senderField(t, MessageSender.AUTOMATION, [
        ...(automations.data?.items ?? []),
        ...old(oldAutomations.data?.items),
      ]),
    ]
  }, [
    message,
    archived,
    broadcasts.data,
    automations.data,
    oldBroadcasts.data,
    oldAutomations.data,
    t,
  ])

  return (
    <Stack gap="xs">
      {message ? (
        <Checkbox
          label={t(($) => $.conditions.showArchivedSenders)}
          checked={archived}
          onChange={(event) => setArchived(event.currentTarget.checked)}
        />
      ) : null}
      <ConditionBuilder
        workspaceId={workspaceId}
        place="eventFilter"
        initialValue={initialValue}
        propertyValues={offered}
        propertyKeys={keys.data?.keys ?? []}
        filterFields={senderFields}
        onChange={(draft) => onChange(subjectless(draft))}
      />
    </Stack>
  )
}
