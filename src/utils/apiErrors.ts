import { ProblemCode } from '../generated/site/types.gen.ts'

export type ApiErrorLike = {
  detail?: string | null
  form?: string | null
  title?: string | null
  error?: string | null
  status?: number | null
  retryAfter?: number | null
  code?: string | null
}

function isApiErrorLike(error: unknown): error is ApiErrorLike {
  return typeof error === 'object' && error !== null
}

// A 403 problem+json body carries `status: 403` (RFC 7807); the backend answers
// it for role-gated actions a plain member is not allowed to perform.
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

// A 403 problem with code email_verification_required: login refused an Unverified
// User until the emailed link is followed (ADR 0030).
export function isEmailVerificationRequiredError(error: unknown) {
  return (
    isApiErrorLike(error) &&
    error.status === 403 &&
    error.code === ProblemCode.EMAIL_VERIFICATION_REQUIRED
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
  if (!isApiErrorLike(error)) return undefined
  const { retryAfter } = error
  return typeof retryAfter === 'number' && retryAfter > 0 ? retryAfter : undefined
}

// How long a 429 asks the caller to wait, as a whole count of one unit: minutes
// (rounded up) from a minute on, seconds below. Undefined when the error is not a
// 429 or carries no wait. Screens pick their own localized sentence from it.
export function getRateLimitWait(error: unknown) {
  const seconds = getRetryAfterSeconds(error)
  if (!isRateLimitedError(error) || seconds === undefined) return undefined
  return seconds >= 60
    ? ({ unit: 'minutes', count: Math.ceil(seconds / 60) } as const)
    : ({ unit: 'seconds', count: seconds } as const)
}

export function getApiErrorMessage(error: unknown, fallback: string) {
  if (!isApiErrorLike(error)) return fallback
  return error.detail ?? error.form ?? error.error ?? error.title ?? fallback
}
