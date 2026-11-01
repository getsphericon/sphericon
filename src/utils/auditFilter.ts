import { z } from 'zod'

import type { SiteAuditActorKind } from '../generated/site/types.gen.ts'
import { zAuditAction, zSiteAuditActorKind } from '../generated/site/zod.gen.ts'

// The Audit log filter lives in the settings route's search, so a Change history link
// (and a reload) opens the log already narrowed. Every field is optional text; `from`
// and `to` are calendar dates (YYYY-MM-DD). An actor kind or an action the contract does
// not know (an old link) is dropped rather than sent.
// Coerced because the router parses an all-digit value (an id) back to a number.
const optionalText = z.coerce.string().optional().catch(undefined)

export const auditFilterSchema = z.object({
  from: optionalText,
  to: optionalText,
  actorKind: zSiteAuditActorKind.optional().catch(undefined),
  actorId: optionalText,
  action: zAuditAction.optional().catch(undefined),
  targetType: optionalText,
  targetId: optionalText,
  ip: optionalText,
  requestId: optionalText,
})

export type AuditFilter = z.infer<typeof auditFilterSchema>

// The Audit entry target types a Change history link can point at.
export const AUDIT_TARGET_INTEGRATION = 'integration'
export const AUDIT_TARGET_WEBHOOK_ENDPOINT = 'webhook_endpoint'

function startOfDay(date: string) {
  const [year, month, day] = date.split('-').map(Number)
  return new Date(year ?? 0, (month ?? 1) - 1, day ?? 1)
}

// auditQuery maps the filter to the API's query: the period's days become an inclusive
// start and an exclusive end (the start of the day after `to`), in the viewer's timezone.
export function auditQuery(filter: AuditFilter) {
  const to = filter.to ? startOfDay(filter.to) : undefined
  if (to) {
    to.setDate(to.getDate() + 1)
  }
  return {
    ...(filter.from ? { from: startOfDay(filter.from).toISOString() } : {}),
    ...(to ? { to: to.toISOString() } : {}),
    ...(filter.actorKind ? { actorKind: filter.actorKind } : {}),
    ...(filter.actorId ? { actorId: filter.actorId } : {}),
    ...(filter.action ? { action: filter.action } : {}),
    ...(filter.targetType ? { targetType: filter.targetType } : {}),
    ...(filter.targetId ? { targetId: filter.targetId } : {}),
    ...(filter.ip ? { ip: filter.ip } : {}),
    ...(filter.requestId ? { requestId: filter.requestId } : {}),
  }
}

// An actor as the "By" filter offers it: a kind, and an id unless the kind has none.
interface AuditActorRef {
  kind: SiteAuditActorKind
  id?: string | null | undefined
}

// actorValue is the "By" select's value of an actor: its kind, then its id when it has one.
export function actorValue(actor: AuditActorRef) {
  return actor.id ? `${actor.kind}:${actor.id}` : actor.kind
}

// filterActorValue is the "By" select's value for the actor the filter narrows to.
export function filterActorValue(filter: AuditFilter) {
  return filter.actorKind ? actorValue({ kind: filter.actorKind, id: filter.actorId }) : null
}

// withActor narrows the filter to the actor of a "By" select value, or, for none or a value
// it cannot read, to every actor.
export function withActor(filter: AuditFilter, value: string | null): AuditFilter {
  const { actorKind: _kind, actorId: _id, ...rest } = filter
  const separator = value?.indexOf(':') ?? -1
  const kind = zSiteAuditActorKind.safeParse(separator < 0 ? value : value?.slice(0, separator))
  if (!kind.success) {
    return rest
  }
  const id = separator < 0 ? undefined : value?.slice(separator + 1)
  return { ...rest, actorKind: kind.data, ...(id ? { actorId: id } : {}) }
}

// withPeriod narrows the filter to the days of a picked range. A range picked by half (its
// first day only) is not a period yet: it answers undefined and the filter stays as it is.
export function withPeriod(
  filter: AuditFilter,
  [from, to]: [string | null, string | null],
): AuditFilter | undefined {
  const { from: _from, to: _to, ...rest } = filter
  if (from && to) {
    return { ...rest, from, to }
  }
  return from || to ? undefined : rest
}

// The filters that narrow to one value no select offers: set by following a link or by
// clicking a value in an entry, and shown as chips that remove them.
const CHIP_FIELDS = ['actorId', 'targetId', 'ip', 'requestId'] as const

export type AuditChipField = (typeof CHIP_FIELDS)[number]

// filterChips lists the filter's chip fields that are set. An actor id the "By" select
// already shows (its kind is set too) is no chip.
export function filterChips(filter: AuditFilter) {
  return CHIP_FIELDS.flatMap((field) => {
    const value = filter[field]
    const shownBySelect = field === 'actorId' && filter.actorKind
    return value && !shownBySelect ? [{ field, value }] : []
  })
}

// withAction narrows the filter to the action of the action select's value, or, for none or
// a value the contract does not know, to every action.
export function withAction(filter: AuditFilter, value: string | null): AuditFilter {
  const { action: _old, ...rest } = filter
  const action = zAuditAction.safeParse(value)
  return action.success ? { ...rest, action: action.data } : rest
}

// withValue narrows the filter by one field, or, for an empty value, stops narrowing by it.
export function withValue(
  filter: AuditFilter,
  field: 'targetType' | AuditChipField,
  value: string | null | undefined,
): AuditFilter {
  const { [field]: _old, ...rest } = filter
  return value ? { ...rest, [field]: value } : rest
}
