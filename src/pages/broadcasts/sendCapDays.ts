import type { SiteSendCap } from '../../generated/site/types.gen.ts'

// How many days a Broadcast to `audience` contacts takes under the Send cap (ADR 0067), by the
// server's model of its bucket: what the bucket holds goes out at once, the rest at the per-day
// value per 24 hours. The client does not see the bucket, so it takes per-day less the sends of the
// last 24 hours (negative after Transactional sends beyond it): no more than the bucket holds, so
// the estimate errs late. Null when no Send cap applies or the audience goes out within 24 hours,
// so there is nothing to warn about.
export function sendCapDays(audience: number, sendCap: SiteSendCap | null | undefined) {
  if (!sendCap) {
    return null
  }
  const days = (audience - (sendCap.perDay - sendCap.sentLast24h)) / sendCap.perDay
  return days > 1 ? Math.ceil(days) : null
}
