import { ProblemCode } from '../generated/site/types.gen.ts'
import type { ProblemDetails } from '../generated/site/types.gen.ts'
import {
  zInUseProblemDetails,
  zProblemDetails,
  zSiteDeleteUserProblemDetails,
} from '../generated/site/zod.gen.ts'

// What a failed call of the generated client rejects with: a problem body (RFC 7807) as the
// contract describes it. Anything else (a network failure, a page that threw) is not one.
export type ApiErrorLike = ProblemDetails

function asProblem(error: unknown): ProblemDetails | undefined {
  const parsed = zProblemDetails.safeParse(error)
  return parsed.success ? parsed.data : undefined
}

// A 403 problem+json body carries `status: 403` (RFC 7807); the backend answers
// it for role-gated actions a plain member is not allowed to perform.
// Whether the error is a problem body answering one of the given HTTP statuses.
export function hasErrorStatus(error: unknown, statuses: readonly number[]) {
  return statuses.includes(asProblem(error)?.status ?? 0)
}

export function isForbiddenError(error: unknown) {
  return asProblem(error)?.status === 403
}

// Whether the error is a problem answering the status with the code.
function isProblem(error: unknown, status: number, code: ProblemCode) {
  const problem = asProblem(error)
  return problem?.status === status && problem.code === code
}

// A 403 problem with code second_factor_required: the Workspace requires a Second
// factor and the User's grace is over (ADR 0020). Any request to that Workspace
// answers it until the User enrolls one.
export function isSecondFactorRequiredError(error: unknown) {
  return isProblem(error, 403, ProblemCode.SECOND_FACTOR_REQUIRED)
}

// A 403 problem with code acceptance_pending: a required Sign-up item has a version the User has
// not accepted (ADR 0062), and every page but the accept-or-sign-out screen is refused.
export function isAcceptancePendingError(error: unknown) {
  return isProblem(error, 403, ProblemCode.ACCEPTANCE_PENDING)
}

// A 403 problem with code email_verification_required: login refused an Unverified
// User until the emailed link is followed (ADR 0030).
export function isEmailVerificationRequiredError(error: unknown) {
  return isProblem(error, 403, ProblemCode.EMAIL_VERIFICATION_REQUIRED)
}

// A 503 problem with code migrations_pending: a development server's database has not run the
// migrations its build needs (ADR 0033), and the error page offers to run them.
export function isMigrationsPendingError(error: unknown) {
  return isProblem(error, 503, ProblemCode.MIGRATIONS_PENDING)
}

// A 429 problem body carries `status: 429` (RFC 7807): a rate limit was hit
// (ADR 0025). The generated client types it, so callers branch on the status, not
// on the English detail.
export function isRateLimitedError(error: unknown) {
  return asProblem(error)?.status === 429
}

// How long a 429 asks the caller to wait (`retryAfter`, the body twin of the Retry-After
// header), as a whole count of one unit: minutes (rounded up) from a minute on, seconds below.
// Undefined when the error is not a 429 or carries no wait. Screens pick their own localized
// sentence from it.
export function getRateLimitWait(error: unknown) {
  const problem = asProblem(error)
  const seconds = problem?.retryAfter
  if (problem?.status !== 429 || seconds === undefined || seconds <= 0) {
    return undefined
  }
  return seconds >= 60
    ? ({ unit: 'minutes', count: Math.ceil(seconds / 60) } as const)
    : ({ unit: 'seconds', count: seconds } as const)
}

// What the problem body itself says went wrong, or undefined for an error that carries no
// message, such as a network failure.
export function getProblemMessage(error: unknown) {
  const problem = asProblem(error)
  return problem?.detail ?? problem?.form ?? problem?.title
}

export function getApiErrorMessage(error: unknown, fallback: string) {
  return getProblemMessage(error) ?? fallback
}

// The status line of a problem body ("502 Bad Gateway"), or undefined for an error that is not
// one, such as a page that threw while rendering.
export function getProblemStatusLine(error: unknown) {
  const problem = asProblem(error)
  if (problem?.status === undefined) {
    return undefined
  }
  return problem.title ? `${problem.status} ${problem.title}` : String(problem.status)
}

// What a development server adds to a 500 (the `debug` member of the problem): the error, the
// stack of a panic and the request. A production Instance never sends it.
export function getProblemDebug(error: unknown) {
  return asProblem(error)?.debug
}

// The rows that use the one a refused removal named (the `usedBy` member of a 409, ADR 0049),
// or undefined when the error is not such a refusal.
export function getUsedBy(error: unknown) {
  const parsed = zInUseProblemDetails.safeParse(error)
  if (!parsed.success || parsed.data.status !== 409 || parsed.data.usedBy.length === 0) {
    return undefined
  }
  return parsed.data.usedBy
}

// The Workspaces a refused User deletion named (the `workspaces` member of its 422, ADR 0050):
// the ones the User is the only Owner of. Undefined when the error is not such a refusal.
export function getSoleOwnedWorkspaces(error: unknown) {
  const parsed = zSiteDeleteUserProblemDetails.safeParse(error)
  if (!parsed.success || parsed.data.status !== 422 || parsed.data.workspaces.length === 0) {
    return undefined
  }
  return parsed.data.workspaces
}

// Every message a 422 problem lists, by field (its `errors` member); undefined when it lists none.
export function getFieldErrors(error: unknown) {
  const problem = asProblem(error)
  return problem?.status === 422 ? problem.errors : undefined
}

// The first message a 422 problem lists under one field (its `errors` member), or undefined when
// the error names no such field.
export function getFieldError(error: unknown, field: string) {
  const problem = asProblem(error)
  if (problem?.status !== 422) {
    return undefined
  }
  return problem.errors?.[field]?.[0]
}
