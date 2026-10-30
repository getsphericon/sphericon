import { ProblemCode } from '../generated/site/types.gen.ts'
import type { ProblemDebug, SiteSoleOwnedWorkspace, UsedBy } from '../generated/site/types.gen.ts'

export interface ApiErrorLike {
  detail?: string | null
  form?: string | null
  title?: string | null
  error?: string | null
  status?: number | null
  retryAfter?: number | null
  code?: string | null
  debug?: ProblemDebug | null
  usedBy?: UsedBy[] | null
  workspaces?: SiteSoleOwnedWorkspace[] | null
  errors?: Record<string, string[]> | null
}

function isApiErrorLike(error: unknown): error is ApiErrorLike {
  return typeof error === 'object' && error !== null
}

// A 403 problem+json body carries `status: 403` (RFC 7807); the backend answers
// it for role-gated actions a plain member is not allowed to perform.
// Whether the error is a problem body answering one of the given HTTP statuses.
export function hasErrorStatus(error: unknown, statuses: readonly number[]) {
  return isApiErrorLike(error) && statuses.includes(error.status ?? 0)
}

export function isForbiddenError(error: unknown) {
  return isApiErrorLike(error) && error.status === 403
}

// A 403 problem with code second_factor_required: the Workspace requires a Second
// factor and the User's grace is over (ADR 0020). Any request to that Workspace
// answers it until the User enrolls one.
export function isSecondFactorRequiredError(error: unknown) {
  return (
    isApiErrorLike(error) &&
    error.status === 403 &&
    error.code === ProblemCode.SECOND_FACTOR_REQUIRED
  )
}

// A 403 problem with code acceptance_pending: a required Sign-up item has a version the User has
// not accepted (ADR 0062), and every page but the accept-or-sign-out screen is refused.
export function isAcceptancePendingError(error: unknown) {
  return (
    isApiErrorLike(error) && error.status === 403 && error.code === ProblemCode.ACCEPTANCE_PENDING
  )
}

// A 403 problem with code email_verification_required: login refused an Unverified
// User until the emailed link is followed (ADR 0030).
export function isEmailVerificationRequiredError(error: unknown) {
  return (
    isApiErrorLike(error) &&
    error.status === 403 &&
    error.code === ProblemCode.EMAIL_VERIFICATION_REQUIRED
  )
}

// A 503 problem with code migrations_pending: a development server's database has not run the
// migrations its build needs (ADR 0033), and the error page offers to run them.
export function isMigrationsPendingError(error: unknown) {
  return (
    isApiErrorLike(error) && error.status === 503 && error.code === ProblemCode.MIGRATIONS_PENDING
  )
}

// A 429 problem body carries `status: 429` (RFC 7807): a rate limit was hit
// (ADR 0025). The generated client types it, so callers branch on the status, not
// on the English detail.
export function isRateLimitedError(error: unknown) {
  return isApiErrorLike(error) && error.status === 429
}

// The seconds a 429 problem asks the caller to wait (`retryAfter`, the body twin of
// the Retry-After header), or undefined when the error carries none.
function getRetryAfterSeconds(error: unknown) {
  if (!isApiErrorLike(error)) {
    return undefined
  }
  const { retryAfter } = error
  return typeof retryAfter === 'number' && retryAfter > 0 ? retryAfter : undefined
}

// How long a 429 asks the caller to wait, as a whole count of one unit: minutes
// (rounded up) from a minute on, seconds below. Undefined when the error is not a
// 429 or carries no wait. Screens pick their own localized sentence from it.
export function getRateLimitWait(error: unknown) {
  const seconds = getRetryAfterSeconds(error)
  if (!isRateLimitedError(error) || seconds === undefined) {
    return undefined
  }
  return seconds >= 60
    ? ({ unit: 'minutes', count: Math.ceil(seconds / 60) } as const)
    : ({ unit: 'seconds', count: seconds } as const)
}

export function getApiErrorMessage(error: unknown, fallback: string) {
  if (!isApiErrorLike(error)) {
    return fallback
  }
  return error.detail ?? error.form ?? error.error ?? error.title ?? fallback
}

// The status line of a problem body ("502 Bad Gateway"), or undefined for an error that is not
// one, such as a page that threw while rendering.
export function getProblemStatusLine(error: unknown) {
  if (!isApiErrorLike(error) || typeof error.status !== 'number') {
    return undefined
  }
  return error.title ? `${error.status} ${error.title}` : String(error.status)
}

// What a development server adds to a 500 (the `debug` member of the problem): the error, the
// stack of a panic and the request. A production Instance never sends it.
export function getProblemDebug(error: unknown) {
  return isApiErrorLike(error) ? (error.debug ?? undefined) : undefined
}

// The rows that use the one a refused removal named (the `usedBy` member of a 409, ADR 0049),
// or undefined when the error is not such a refusal.
export function getUsedBy(error: unknown) {
  if (!isApiErrorLike(error) || error.status !== 409 || !error.usedBy?.length) {
    return undefined
  }
  return error.usedBy
}

// The Workspaces a refused User deletion named (the `workspaces` member of its 422, ADR 0050):
// the ones the User is the only Owner of. Undefined when the error is not such a refusal.
export function getSoleOwnedWorkspaces(error: unknown) {
  if (!isApiErrorLike(error) || error.status !== 422 || !error.workspaces?.length) {
    return undefined
  }
  return error.workspaces
}

// The first message a 422 problem lists under one field (its `errors` member), or undefined when
// the error names no such field.
export function getFieldError(error: unknown, field: string) {
  if (!isApiErrorLike(error) || error.status !== 422) {
    return undefined
  }
  return error.errors?.[field]?.[0]
}
