import { z } from 'zod'

// The Audit log filter lives in the settings route's search, so a Change history link
// (and a reload) opens the log already narrowed. Every field is optional text; `from`
// and `to` are calendar dates (YYYY-MM-DD).
// Coerced because the router parses an all-digit value (an id) back to a number.
const optionalText = z.coerce.string().optional().catch(undefined)

export const auditFilterSchema = z.object({
  from: optionalText,
  to: optionalText,
  actorKind: z.enum(['user', 'api_token', 'operator', 'system']).optional().catch(undefined),
  actorId: optionalText,
  action: optionalText,
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
  if (to) to.setDate(to.getDate() + 1)
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
