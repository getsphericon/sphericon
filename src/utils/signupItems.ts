import { SignupItemKind } from '../generated/site/types.gen.ts'
import type {
  SiteSignUpInput,
  SiteSignupItem,
  SiteSignupItemAnswer,
} from '../generated/site/types.gen.ts'
import { SignupItemKind as StaffSignupItemKind } from '../generated/staff/types.gen.ts'
import type { StaffSignupItemResource } from '../generated/staff/types.gen.ts'

// The answers a sign-up form sends for the Sign-up items it showed (ADR 0062): each item at the
// version shown, a box ticked only when the person ticked it, a notice only named. Undefined with
// no items, so the form sends what it sent before items existed.
export function signupItemAnswers(
  items: readonly SiteSignupItem[],
  ticked: Readonly<Record<string, boolean>>,
): SiteSignupItemAnswer[] | undefined {
  if (items.length === 0) {
    return undefined
  }
  return items.map((item) =>
    item.kind === SignupItemKind.NOTICE
      ? { key: item.key, version: item.version }
      : { key: item.key, version: item.version, accepted: ticked[item.key] === true },
  )
}

// The request field that carries the answers, named by the contract: a rename there fails here.
const answersField = 'signupItems' satisfies keyof SiteSignUpInput

// The field a refusal of the item with this key is reported under: the answers field and the key,
// as the contract documents it (`signupItems.<key>`).
export function signupItemField(key: string) {
  return `${answersField}.${key}`
}

// The id of the item's checkbox, so a form can focus the box whose answer was refused.
export function signupItemInputId(key: string) {
  return `signup-item-${key}`
}

// Whether publishing a new version of the item stops every User who has not accepted it (ADR
// 0062): a required acceptance does; a deleted one cannot be published at all.
export function publishStopsUsers(item: Pick<StaffSignupItemResource, 'kind'>) {
  return item.kind === StaffSignupItemKind.REQUIRED_ACCEPTANCE
}
