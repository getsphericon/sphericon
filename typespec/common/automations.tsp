// The Automation models /site and /api show and take the same way: its steps, versions, write
// bodies, HTTP step dry run and report.

import "./engagement.tsp";
import "./local-calendar.tsp";
import "./segments.tsp";
import "./signing-secret.tsp";

namespace Sphericon;

/** Automation lifecycle status: active enrolls Contacts, inactive enrolls nobody */
enum AutomationStatus {
  active,
  inactive,
}

/** One request header of an HTTP step. The value is a secret: it is never returned. */
model AutomationHttpHeader {
  /** Header name. Content-Type, Content-Length, Host and the webhook-* headers are set by sphericon. */
  name: string;

  /** Header value. Omit it when editing to keep the stored value of the header with this name. */
  value?: string;
}

model AutomationHttpHeaderInput is AutomationHttpHeader;

/** Closed error kind of an HTTP step's body */
enum AutomationHttpErrorKind {
  invalid_body,
}

/** Request body for rendering an HTTP step's body for a Contact */
model DryRunHttpStepInput {
  /** The Liquid JSON body to render. No Enrollment stands behind a dry run, so `event` is empty */
  body: string;

  /** The Contact whose values fill the template */
  contactId: EntityId;
}

/** The body rendered for a Contact, or the kind of error that stopped it. Nothing is called. */
model DryRunHttpStepResult {
  /** The rendered, valid JSON body. Absent when errorKind is set. */
  body?: string;

  /** Why the body could not be rendered */
  errorKind?: AutomationHttpErrorKind;

  /** Human-readable detail of the error */
  detail?: string;
}

/** The id of a Step or a path: a UUID in its lowercase text form, kept by every version of the
 * automation. A client mints it when it creates the Step or path and sends it back on every save;
 * one sent without an id is given one. Unique within a version, Steps and paths together. */
scalar StepId extends string;

/** A Step that sends an email through the Integration it names */
model EmailStep {
  /** The Step's id */
  id: StepId;

  type: "email";

  /** Email subject */
  subject: string;

  /** Email body as HTML. Binds the Contact's fields plus `event.action` and `event.properties`: the Event the Enrollment began on (the one a Rule fired on, when a Rule enrolled), copied when it began; empty when no Event began it */
  body: string;

  /** The editor's JSON document the body was built from; absent when the body was written as HTML */
  doc?: EditorDocument;

  /** Version of the editor that rendered the document; absent with it */
  editorVersion?: string;

  /** The Integration the Step sends through, of the email Channel; absent is the Channel's default. Each send step names its own, so one automation may send through several (ADR 0081). */
  integrationId?: EntityId;
}

/** A Step that sends an email, as a request writes it */
model EmailStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "email";

  /** Email subject */
  subject: string;

  /** Email body as HTML, bound as on a stored email Step. Write it or `doc`, not both (both is a 422). Writing only a body clears the Step's document. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the body from it in the same save, and a document that cannot be rendered refuses the whole save, naming the Step. */
  doc?: EditorDocument;

  /** The Integration the Step sends through, of the email Channel; null or omitted is the Channel's default */
  integrationId?: EntityId | null;
}

/** A Step that sends a Telegram message through the Integration it names */
model TelegramStep {
  /** The Step's id */
  id: StepId;

  type: "telegram";

  /** The message: Telegram's HTML subset (b, strong, i, em, u, ins, s, strike, del, a with an http(s) href, code, pre, tg-spoiler, blockquote) with Liquid merge tags, whose values are escaped. With an image, its caption. */
  text: string;

  /** The label of an unsubscribe button under the message; absent for none */
  unsubscribeButton?: string;

  /** The https image sent with the message; absent for a text message */
  imageUrl?: string;

  /** The link buttons under the message, one per row, in order */
  buttons: TelegramButton[];

  /** Whether a link in the text unfolds a preview; no effect with an image */
  linkPreview: boolean;

  /** The Integration the Step sends through, of the Telegram Channel; absent is the Channel's default */
  integrationId?: EntityId;
}

/** A Step that sends a Telegram message, as a request writes it */
model TelegramStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "telegram";

  /** The message, as on a stored Telegram Step. Up to 4096 characters as written, or 1024 with an image (its caption): publishing a longer one is refused. */
  text: string;

  /** The label of an unsubscribe button under the message, up to 64 characters. Pressing it is the person's Channel-wide Telegram opt-out; empty or omitted adds no button. */
  @maxLength(64)
  unsubscribeButton?: string;

  /** An https image sent with the message; the text becomes its caption. Omitted or empty for a text message. */
  imageUrl?: string;

  /** Up to 8 link buttons under the message, one per row, in order. */
  @maxItems(8)
  buttons?: TelegramButtonInput[];

  /** Whether a link in the text unfolds a preview. On when omitted; no effect with an image. */
  linkPreview?: boolean;

  /** The Integration the Step sends through, of the Telegram Channel; null or omitted is the Channel's default */
  integrationId?: EntityId | null;
}

/** A Step that sends an SMS through the Integration it names */
model SmsStep {
  /** The Step's id */
  id: StepId;

  type: "sms";

  /** The text: plain text with Liquid merge tags (the Contact's fields, `event.action` and `event.properties`, `unsubscribe_url`), whose values are written as they are. It is sent to the Contact's phone with nothing appended. */
  text: string;

  /** Whether the links of the text go out as short tracked links, whose clicks record `sms.clicked` (ADR 0083) */
  linkTracking: boolean;

  /** The Integration the Step sends through, of the SMS Channel; absent is the Channel's default */
  integrationId?: EntityId;
}

/** A Step that sends an SMS, as a request writes it */
model SmsStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "sms";

  /** The text, as on a stored SMS Step: each http(s) link a short tracked link on the Tracking domain unless `linkTracking` is false; the save refuses a text over the part limit, its links counted as short links. */
  text: string;

  /** Whether the links of the text go out as short tracked links (ADR 0083). On when omitted. */
  linkTracking?: boolean;

  /** The Integration the Step sends through, of the SMS Channel; null or omitted is the Channel's default */
  integrationId?: EntityId | null;
}

/** A Step that waits before the next one */
model WaitStep {
  /** The Step's id */
  id: StepId;

  type: "wait";

  /** Delay before the next Step, in seconds */
  seconds: int32;
}

/** A Step that waits before the next one, as a request writes it */
model WaitStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "wait";

  /** Delay before the next Step, in seconds; a negative one is refused */
  seconds: int32;
}

/** Wait until the next moment a local time comes, on chosen weekdays or on any day, in the
 * Contact's time zone, or the Workspace time zone when the Contact has no valid one (ADR 0126) */
model WaitUntilTimeOfDay {
  form: "time_of_day";

  /** The local time of day */
  localTime: LocalTime;

  /** The weekdays the local time counts on; empty is every day */
  weekdays: Weekday[];
}

/** Wait until the next moment a local time comes, as a request writes it */
model WaitUntilTimeOfDayInput {
  form: "time_of_day";

  /** The local time of day */
  localTime: LocalTime;

  /** The weekdays the local time counts on, each at most once; omitted or empty is every day */
  weekdays?: Weekday[];
}

/** Wait until the next occurrence of a date the Contact holds (ADR 0126), at or after the moment
 * the Enrollment reaches the Step: `offsetDays` before or after the date, at `localTime` in the
 * Contact's time zone, or the Workspace time zone when the Contact has no valid one. A yearly date
 * waits for its next anniversary, from the year after the date's own; a once date for the date
 * itself. When there is no such moment within 366 days the Enrollment goes on at once, and the
 * report counts why: the moment has passed (`past`), the Contact holds no date in the field
 * (`no_date`), or it lies more than 366 days ahead (`beyond_bound`). */
model WaitUntilDate {
  form: "date";
  ...DateOccurrence;
}

/** Wait until the next occurrence of a date the Contact holds, as a request writes it */
model WaitUntilDateInput {
  form: "date";
  ...DateOccurrenceInput;
}

/** The moment a Wait until holds an Enrollment until, by `form` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "form" })
union WaitUntilMoment {
  time_of_day: WaitUntilTimeOfDay,
  date: WaitUntilDate,
}

/** The moment a Wait until holds an Enrollment until, by `form`, as a request writes it */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "form" })
union WaitUntilMomentInput {
  time_of_day: WaitUntilTimeOfDayInput,
  date: WaitUntilDateInput,
}

/** A Step that holds the Enrollment until a calendar moment (ADR 0126): the moment is computed
 * once, from when the Enrollment reaches the Step, and kept, so neither a restart nor a time zone
 * changed during the wait moves it. A moment equal to the arrival waits zero. */
model WaitUntilStep {
  /** The Step's id */
  id: StepId;

  type: "wait_until";

  /** The moment the Enrollment waits for */
  until: WaitUntilMoment;
}

/** A Step that holds the Enrollment until a calendar moment, as a request writes it */
model WaitUntilStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "wait_until";

  /** The moment the Enrollment waits for */
  until: WaitUntilMomentInput;
}

/** A Step that applies a Tag to the enrolled Contact */
model ApplyTagStep {
  /** The Step's id */
  id: StepId;

  type: "apply_tag";

  /** The Tag's name */
  tag: string;
}

/** A Step that applies a Tag, as a request writes it */
model ApplyTagStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "apply_tag";

  /** The Tag's name; a blank one is refused */
  tag: string;
}

/** A Step that removes a Tag from the enrolled Contact */
model RemoveTagStep {
  /** The Step's id */
  id: StepId;

  type: "remove_tag";

  /** The Tag's name */
  tag: string;
}

/** A Step that removes a Tag, as a request writes it */
model RemoveTagStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "remove_tag";

  /** The Tag's name; a blank one is refused */
  tag: string;
}

/** A Step that POSTs a JSON body to a hook URL (ADR 0039). Its URL, header values and signing secret are secrets: never returned. */
model HttpStep {
  /** The Step's id: send it back on edit so the Step keeps its secrets */
  id: StepId;

  type: "http";

  /** The Liquid JSON request body, bound as an email Step's body is */
  body: string;

  /** The request headers, by name only */
  headers: AutomationHttpHeader[];

  /** Whether a signing secret is stored */
  hasSecret: boolean;
}

/** A Step that POSTs a JSON body to a hook URL, as a request writes it. A secret omitted keeps the stored value of the Step with the same id. */
model HttpStepInput {
  /** The Step's id; omitted, the Step is given one. Send the id it was read with so the Step keeps its secrets. */
  id?: StepId;

  type: "http";

  /** The Liquid JSON request body: strings JSON-escaped, up to 64 KiB, must render to valid JSON, also with every tag empty */
  body: string;

  /** Hook URL to POST to. Omit it when editing to keep the stored URL. Up to 2 KiB, http or https. */
  url?: string;

  /** Up to 5 request headers */
  @maxItems(5)
  headers?: AutomationHttpHeaderInput[];

  /** Signing secret: when set, the request is signed with Standard Webhooks. Omit it when editing to keep the stored secret. */
  secret?: SigningSecret;

  /** Remove the signing secret on save */
  clearSecret?: boolean;
}

/** A Step that finishes its path and ends the Enrollment as `completed` at once (ADR 0124): a path
 * stops early by design, which is no exit. It is the last Step of its path. */
model EndStep {
  /** The Step's id */
  id: StepId;

  type: "end";
}

/** A Step that finishes its path, as a request writes it. A Step after it in the same path refuses
 * the save, naming that Step. */
model EndStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "end";
}

/** A path of a Branch: the Condition that sends an Enrollment down it and its Steps */
model BranchPath {
  /** The path's id */
  id: StepId;

  /** The Condition an Enrollment takes the path on, when no path above it is taken */
  condition: BranchCondition;

  /** The path's Steps, in order; none goes on to the Step after the Branch */
  steps: PathStep[];
}

/** A path of a Branch, as a request writes it */
model BranchPathInput {
  /** The path's id; omitted, the path is given one */
  id?: StepId;

  /** The Condition an Enrollment takes the path on, when no path above it is taken. A path
   * without a test refuses the save, naming the Branch. */
  condition: BranchConditionInput;

  /** The path's Steps, in order; none goes on to the Step after the Branch */
  steps: PathStepInput[];
}

/** A Step that sends each Enrollment down the first of its paths whose Condition holds, on the
 * Contact's live data when the Enrollment reaches it (ADR 0124), and down `otherwise` when none
 * does. A Condition that is unknown for the Contact (an engagement count of a Destination that is
 * not tracked, ADR 0123) before any holds sends it down `unknown`. The paths rejoin at the Step
 * after the Branch. */
model BranchStep {
  /** The Step's id */
  id: StepId;

  type: "branch";

  /** The paths, in the order their Conditions are tested */
  paths: BranchPath[];

  /** The path taken when no Condition holds */
  otherwise: AutomationPath;

  /** The path taken when the first Condition that does not fail is unknown; present only on a
   * Branch whose Conditions can be unknown */
  `unknown`?: AutomationPath;
}

/** A Branch, as a request writes it. Publish refuses it without `unknown` when a Condition counts
 * an open or a click, and with `unknown` when none does. */
model BranchStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "branch";

  /** The paths, in the order their Conditions are tested: one to nine, so that with `otherwise` a
   * Branch has at most ten (`unknown` is not counted) */
  @minItems(1)
  @maxItems(9)
  paths: BranchPathInput[];

  /** The path taken when no Condition holds */
  otherwise: AutomationPathInput;

  /** The path taken when the first Condition that does not fail is unknown */
  `unknown`?: AutomationPathInput;
}

/** A path of a Split: its share of the Contacts and its Steps */
model SplitPath {
  /** The path's id */
  id: StepId;

  /** The path's share of the Contacts that reach the Split, a whole percentage */
  weight: int32;

  /** The path's Steps, in order; none is a holdout, sent nothing, that goes on to the Step after
   * the Split */
  steps: SplitPathStep[];
}

/** A path of a Split, as a request writes it */
model SplitPathInput {
  /** The path's id; omitted, the path is given one */
  id?: StepId;

  /** The path's share of the Contacts that reach the Split, a whole percentage. The weights of a
   * Split's paths sum to 100, or the save is refused, naming the Split. */
  @minValue(1)
  weight: int32;

  /** The path's Steps, in order; none is a holdout, sent nothing, that goes on to the Step after
   * the Split */
  steps: SplitPathStepInput[];
}

/** A Step that sends each Enrollment down one of its paths by weight (ADR 0124): a hash of the
 * Contact, the Split and the Enrollment's ordinal chooses the path, so the same Enrollment always
 * takes the same path and a new Enrollment of the Contact draws again. In an Automation with a
 * yearly date Trigger the anniversary year takes the ordinal's place, so the Contact draws again
 * each year (ADR 0126). An empty path is a holdout. The paths rejoin at the Step after the Split. */
model SplitStep {
  /** The Step's id */
  id: StepId;

  type: "split";

  /** The paths, in order */
  paths: SplitPath[];
}

/** A Split, as a request writes it. Its paths' weights sum to 100. */
model SplitStepInput {
  /** The Step's id; omitted, the Step is given one */
  id?: StepId;

  type: "split";

  /** The paths, in order: two to ten */
  @minItems(2)
  @maxItems(10)
  paths: SplitPathInput[];
}

/** One Step of an automation, by `type` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union AutomationStep {
  email: EmailStep,
  telegram: TelegramStep,
  sms: SmsStep,
  wait: WaitStep,
  wait_until: WaitUntilStep,
  apply_tag: ApplyTagStep,
  remove_tag: RemoveTagStep,
  http: HttpStep,
  end: EndStep,
  branch: BranchStep,
  split: SplitStep,
}

/** One Step of an automation, by `type`, as a request writes it. A field of another kind is refused. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union AutomationStepInput {
  email: EmailStepInput,
  telegram: TelegramStepInput,
  sms: SmsStepInput,
  wait: WaitStepInput,
  wait_until: WaitUntilStepInput,
  apply_tag: ApplyTagStepInput,
  remove_tag: RemoveTagStepInput,
  http: HttpStepInput,
  end: EndStepInput,
  branch: BranchStepInput,
  split: SplitStepInput,
}

// A fork's listed paths hold their Steps through a union of their own, the same arms as
// AutomationStep: ogen breaks a recursion only at an object, so the union a fork's path reaches
// must be one that no Step union above it is still being built from. Each fork kind has its own:
// a union shared by two forks is reached again, through the other fork, while it is being built.

/** One Step of a Branch's listed path, by `type`: the arms of AutomationStep */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union PathStep {
  email: EmailStep,
  telegram: TelegramStep,
  sms: SmsStep,
  wait: WaitStep,
  wait_until: WaitUntilStep,
  apply_tag: ApplyTagStep,
  remove_tag: RemoveTagStep,
  http: HttpStep,
  end: EndStep,
  branch: BranchStep,
  split: SplitStep,
}

/** One Step of a Branch's listed path, by `type`, as a request writes it: the arms of AutomationStepInput */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union PathStepInput {
  email: EmailStepInput,
  telegram: TelegramStepInput,
  sms: SmsStepInput,
  wait: WaitStepInput,
  wait_until: WaitUntilStepInput,
  apply_tag: ApplyTagStepInput,
  remove_tag: RemoveTagStepInput,
  http: HttpStepInput,
  end: EndStepInput,
  branch: BranchStepInput,
  split: SplitStepInput,
}

/** One Step of a Split's path, by `type`: the arms of AutomationStep */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union SplitPathStep {
  email: EmailStep,
  telegram: TelegramStep,
  sms: SmsStep,
  wait: WaitStep,
  wait_until: WaitUntilStep,
  apply_tag: ApplyTagStep,
  remove_tag: RemoveTagStep,
  http: HttpStep,
  end: EndStep,
  branch: BranchStep,
  split: SplitStep,
}

/** One Step of a Split's path, by `type`, as a request writes it: the arms of AutomationStepInput */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union SplitPathStepInput {
  email: EmailStepInput,
  telegram: TelegramStepInput,
  sms: SmsStepInput,
  wait: WaitStepInput,
  wait_until: WaitUntilStepInput,
  apply_tag: ApplyTagStepInput,
  remove_tag: RemoveTagStepInput,
  http: HttpStepInput,
  end: EndStepInput,
  branch: BranchStepInput,
  split: SplitStepInput,
}

/** A path of Steps, in the order an Enrollment walks them. A version's Steps are its root path. */
model AutomationPath {
  /** The path's id */
  id: StepId;

  /** The path's Steps, in order */
  steps: AutomationStep[];
}

/** A path of Steps, as a request writes it */
model AutomationPathInput {
  /** The path's id; omitted, the path is given one */
  id?: StepId;

  /** The path's Steps, in order */
  steps: AutomationStepInput[];
}

/** Whether an automation enrolls a contact again (ADR 0127): `once` enrolls a contact at most once,
 * ever; `again` enrolls it again once its previous enrollment ended, however it ended, and the
 * cooldown from that enrollment's beginning passed. Either way a contact has at most one active
 * enrollment in an automation, or one per value of its Enrollment key, by which the policy and the
 * cooldown then count. */
enum EntryPolicy {
  once,
  again,
}

/** The least time, in whole seconds, from the beginning of a contact's previous enrollment in an
 * automation to the beginning of the next, under the `again` entry policy: at least one hour */
@minValue(3600)
scalar EntryCooldown extends int32;

/** A Trigger that enrolls a Contact on an Event (ADR 0133): its action, and an optional filter on
 * that Event's own properties, so `order.completed` where `status` is `paid` starts on paid orders
 * only. An Event that fails the filter is no entry: it enrolls no one and is not a skipped entry. */
model EventTrigger {
  kind: "event";

  /** The Event action that enrolls (e.g. "contact.created", "email.opened", "order.completed").
   * Any customer action; of the email events only opened, clicked and unsubscribed. */
  action: string;

  /** The tests of the Event's own properties it must pass; absent, every Event of the action
   * enrolls. The `tag` of contact.tagged and contact.untagged and the `key` of
   * contact.field_changed, tested by eq or neq, must name a Tag or Custom field of the Workspace. On
   * a message's sent, delivery and engagement Events it may name the Broadcast or Automation that
   * sent the message (`sentBy`). */
  filter?: EventFilter;

  /** Whether a contact enrolls again, after its previous enrollment ended, on a later Event or a
   * rule's enroll action */
  entryPolicy: EntryPolicy;

  /** Under `again`, the least time in seconds from the beginning of a contact's previous
   * enrollment to the next; an entry sooner is skipped as `cooldown` */
  cooldown: EntryCooldown;

  /** The Enrollment key (ADR 0165): a property of the triggering Event, such as `order_id`, by
   * whose value a contact may have several active enrollments at once, one per value; the entry
   * policy and the cooldown count per value. An Event without the property enrolls no one and is
   * skipped as `no_enrollment_key`; a contact has at most 10 active enrollments and 10 begun in a
   * day, past which an entry is skipped as `enrollment_ceiling`. An exit or goal Event carrying the
   * property ends only the enrollment of its value. Absent, a contact has one active enrollment. */
  enrollmentKey?: string;
}

/** A Trigger that enrolls a Contact on an Event (ADR 0133), as a request writes it */
model EventTriggerInput {
  kind: "event";

  /** The Event action that enrolls (e.g. "contact.created", "email.opened", "order.completed").
   * Any customer action; of the email events only opened, clicked and unsubscribed. */
  action: string;

  /** The tests of the Event's own properties it must pass; omitted, every Event of the action
   * enrolls. The `tag` of contact.tagged and contact.untagged and the `key` of
   * contact.field_changed, tested by eq or neq, must name a Tag or Custom field of the Workspace. On
   * a message's sent, delivery and engagement Events it may name the Broadcast or Automation that
   * sent the message (`sentBy`). */
  filter?: EventFilterInput;

  /** Whether a contact enrolls again, after its previous enrollment ended, on a later Event or a
   * rule's enroll action; omitted is `once` */
  entryPolicy?: EntryPolicy = EntryPolicy.once;

  /** Under `again`, the least time in seconds from the beginning of a contact's previous
   * enrollment to the next, at least 3600; omitted is 3600 */
  cooldown?: EntryCooldown = 3600;

  /** The Enrollment key (ADR 0165): the name of a property of the triggering Event, such as
   * `order_id`, by whose value a contact may have several active enrollments at once, one per
   * value; a property name, checked for form only. Omitted, a contact has one active enrollment. */
  enrollmentKey?: string;
}

/** A Trigger that enrolls a Contact on a date it holds (ADR 0126): on each anniversary of its
 * birthday or of a datetime Custom field, or once on such a field, `offsetDays` before or after it,
 * at `localTime` in the Contact's time zone, or in the Workspace time zone when the Contact has no
 * valid one. A yearly date enrolls a Contact at most once per anniversary year, the year of the date
 * the offset counts from, and a once date at most once ever; an occurrence while the previous
 * enrollment is still active is skipped. Only occurrences after the automation was published
 * enroll, at most 24 hours late, and the enrollment begins at the occurrence's moment with no
 * triggering Event. A date Trigger has no entry policy, cooldown or Enrollment key, and no rule's
 * enroll action may name its automation. */
model DateTrigger {
  kind: "date";
  ...DateOccurrence;
}

/** A Trigger that enrolls a Contact on a date it holds (ADR 0126), as a request writes it */
model DateTriggerInput {
  kind: "date";
  ...DateOccurrenceInput;
}

/** What enrolls a Contact in an automation, by `kind`: an Event matcher or a date */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AutomationTrigger {
  event: EventTrigger,
  date: DateTrigger,
}

/** What enrolls a Contact in an automation, by `kind`: an Event matcher or a date */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union AutomationTriggerInput {
  event: EventTriggerInput,
  date: DateTriggerInput,
}

/** An Event matcher that ends an enrollment (ADR 0133): an action, and an optional filter on that
 * Event's own properties, so `order.closed` where `status` is `cancelled` exits and the same action
 * where `status` is `paid` reaches the goal. The Event that began the enrollment never ends it. */
model EventMatcher {
  /** The Event action (e.g. "plan_downgraded", "contact.tagged"). Any customer action; of the
   * email events only opened, clicked and unsubscribed. */
  action: string;

  /** The tests of the Event's own properties it must pass; absent, every Event of the action
   * matches. The `tag` of contact.tagged and contact.untagged and the `key` of
   * contact.field_changed, tested by eq or neq, must name a Tag or Custom field of the Workspace. On
   * a message's sent, delivery and engagement Events it may name the Broadcast or Automation that
   * sent the message (`sentBy`). */
  filter?: EventFilter;
}

/** An Event matcher that ends an enrollment (ADR 0133), as a request writes it */
model EventMatcherInput {
  /** The Event action (e.g. "plan_downgraded", "contact.tagged"). Any customer action; of the
   * email events only opened, clicked and unsubscribed. */
  action: string;

  /** The tests of the Event's own properties it must pass; omitted, every Event of the action
   * matches. The `tag` of contact.tagged and contact.untagged and the `key` of
   * contact.field_changed, tested by eq or neq, must name a Tag or Custom field of the Workspace. On
   * a message's sent, delivery and engagement Events it may name the Broadcast or Automation that
   * sent the message (`sentBy`). */
  filter?: EventFilterInput;
}

/** One version of an automation's definition: what publishing makes it do (ADR 0065) */
model AutomationVersion {
  /** 1 for the first version, plus one per version after it */
  number: int32;

  /** What enrolls a contact */
  trigger: AutomationTrigger;

  /** The Event matchers that end an active enrollment early, as exited. Only an Event that
   * occurred once the enrollment began counts; machine opens and clicks never do. */
  exitEvents: EventMatcher[];

  /** The Event matcher that ends an active enrollment as goal_reached; an Event that matches it and
   * an exit reaches the goal. Null when there is none. */
  goalEvent: EventMatcher | null;

  /** The version's Steps: its root path */
  steps: AutomationPath;
}

/** Request body for creating an automation (always created inactive) */
model CreateAutomationInput {
  name: Name;

  /** What enrolls a contact */
  trigger: AutomationTriggerInput;

  /** The Event matchers that end an active enrollment early. They may share an action with the
   * trigger, with each other and with the goal. */
  exitEvents?: EventMatcherInput[];

  /** The Event matcher that ends an active enrollment as goal_reached; it may share an action with
   * the trigger and the exits. Null or omitted is none. */
  goalEvent?: EventMatcherInput | null;

  /** The version's Steps: its root path */
  steps?: AutomationPathInput;
}

/** Request body for updating an automation. The name changes at once; the trigger, exit and
 * goal events and the steps write the draft (ADR 0065): omitted fields keep the draft's, else the live
 * version's, and only publishing makes the draft live. */
model UpdateAutomationInput {
  /** The automation's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  name?: Name;

  /** What enrolls a contact; replaces the Trigger, its filter, entry policy, cooldown and
   * Enrollment key with it, and omitted keeps it */
  trigger?: AutomationTriggerInput;

  /** The Event matchers that end an active enrollment early; replaces the list, omitted keeps it */
  exitEvents?: EventMatcherInput[];

  /** The Event matcher that ends an active enrollment as goal_reached; null removes it, omitted
   * keeps it */
  goalEvent?: EventMatcherInput | null;

  /** The version's Steps: its root path */
  steps?: AutomationPathInput;
}

/** The length of the period an automation's results are read over, in Report days (ADR 0057), today included */
enum AutomationReportPeriod {
  `7d`,
  `14d`,
  `30d`,
  `90d`,
}

/** How many enrollments began, and how many ended with each outcome */
model AutomationResultCounts {
  /** Enrollments that began: their triggering Event occurred in it */
  entered: int32;

  /** Enrollments that walked every step and ended in it */
  completed: int32;

  /** Enrollments that ended in it because their goal event was recorded */
  goalReached: int32;

  /** Enrollments that left early in it: an unsubscribe from the automation or an exit event. A
   * skipped send (no address, an opt-out from the Channel, a suppression) moves on instead. */
  exited: int32;

  /** Enrollments that ended in it on a step that failed */
  failed: int32;
}

/**
 * One period of an automation's results. Each count is what happened in the period: an
 * enrollment counts as entered on the Report day it began and under its outcome on the Report day
 * it ended, whenever it began, so an enrollment still running counts as entered only.
 */
model AutomationPeriodResults {
  ...AutomationResultCounts;

  /** The period's first Report day, `YYYY-MM-DD` in the Workspace time zone */
  from: string;

  /** The period's last Report day, included */
  to: string;

  /** Distinct contacts among the enrollments that began in the period: fewer than `entered` when a
   * contact entered more than once */
  contacts: int32;

  /** Entries the entry checks refused in the period, by reason (ADR 0127), each on the Report day of
   * the moment it would have begun */
  skipped: AutomationSkippedCounts;

  /** Mean time in whole seconds from the beginning to the end of the enrollments that ended in
   * the period, whatever their outcome; null when none ended */
  averageDurationSeconds: int64 | null;

  /** The orders credited to the automation that were completed in the period, and the net change
   * of its revenue in the period: every Movement of a credited Order counts on its own day (ADR 0064) */
  revenue: MessageRevenue;
}

/** How many entries into an automation were skipped, by reason */
model AutomationSkippedCounts {
  /** Entries caused by a chain of events that already passed through the automation */
  cycle: int32;

  /** Entries whose Event occurred while an enrollment of the contact was active */
  activeEnrollment: int32;

  /** Entries of a contact enrolled in the automation before, which enrolls a contact once */
  once: int32;

  /** Entries sooner after the beginning of the contact's previous enrollment than the cooldown */
  cooldown: int32;

  /** Entries whose Event lacked the property the automation's Enrollment key names */
  noEnrollmentKey: int32;

  /** Entries of a contact that had 10 active enrollments in the keyed automation, or 10 begun in a
   * day */
  enrollmentCeiling: int32;

  /** Occurrences of a date Trigger (ADR 0126) the scheduler reached 24 hours late or more while
   * their date was already set at their moment, which enroll no one; a date set after its
   * occurrence counts nothing */
  missed: int32;

  /** Occurrences of a date Trigger whose date was set through a Connection without mcp:send, which
   * enroll no one while it stays so; a later write of another value without the mark may still enroll the same
   * occurrence within 24 hours, which then counts as entered too */
  sendLocked: int32;
}

/** One Report day of an automation's results */
model AutomationResultsPoint {
  ...AutomationResultCounts;

  /** The Report day, `YYYY-MM-DD` in the Workspace time zone */
  date: string;
}

/** An automation's results over a period of Report days, read from its enrollments, with the
 * period just before it to compare with */
model AutomationResults {
  /** The period the results are read over */
  period: AutomationReportPeriod;

  /** The period ending today */
  current: AutomationPeriodResults;

  /** As many Report days just before it */
  previous: AutomationPeriodResults;

  /** The current period by Report day, oldest first, a day with nothing at zero */
  series: AutomationResultsPoint[];
}

/** An automation's engagement figures over its whole history and its results over a period */
model AutomationReport {
  /** Opens recorded on the automation's emails, every open counted; bounded by Event retention */
  opens: int32;

  /** Clicks recorded on the automation's emails, every click counted; bounded by Event retention */
  clicks: int32;

  /** Messages the provider reported delivered, read from the delivery result stamped on each message; not bounded by Event retention */
  delivered: int32;

  /** Messages the provider reported bounced, read from the delivery result stamped on each message; not bounded by Event retention */
  bounced: int32;

  /** Messages a recipient complained about, read from the delivery result stamped on each message; not bounded by Event retention */
  complained: int32;

  /** Messages that went through a Test inbox: captured, not delivered; they count as delivered too */
  captured: int32;

  /** Orders credited to the automation and their net revenue, over its whole history (ADR 0064) */
  revenue: MessageRevenue;

  /** One entry per clicked link, most clicked first */
  links: LinkClicks[];

  /** What the automation's enrollments did over the chosen period and the period before it */
  results: AutomationResults;

  /** The send Steps' messages by Step id, each summed over every version that had the Step and
   * sorted by the newest of those versions, then by its place there: one entry per Step any of the
   * automation's messages was sent from */
  steps: AutomationStepFigures[];

  /** The send Steps' messages by version and Step id, in version order and, within a version, in
   * its Step order */
  versionSteps: AutomationVersionStepFigures[];

  /** Each path of a Branch (its unknown path included) and of a Split by its fork's Step id and
   * its own id: the enrollments that took it and how many of them reached the goal, over the
   * automation's whole history, each summed over every version that had the path. Sorted by the
   * newest of those versions, then by the fork's place there and the path's place in the fork:
   * one entry per path an enrollment took, so a path none took has none */
  paths: AutomationPathFigures[];

  /** Each Wait until by its Step id, summed over every version that had it: the enrollments that
   * left it in the current period without waiting, because its date had no moment ahead within 366
   * days (ADR 0126). In Step id order: one entry per Wait until any enrollment left so, so one that
   * every enrollment waited on has none */
  waitUntilSteps: AutomationWaitUntilFigures[];
}

/** The enrollments that went on at once from one Wait until on a date instead of waiting, by why:
 * read from what each kept when it reached the Step, counted on the moment it left it */
model AutomationWaitUntilFigures {
  /** The Wait until's Step id, the same in every version */
  stepId: StepId;

  /** Its moment had passed: a once date before the arrival */
  past: int32;

  /** The contact held no date in the field, or a value that is no date */
  noDate: int32;

  /** Its moment lay more than 366 days ahead, such as a yearly date whose value is itself in the
   * future */
  beyondBound: int32;
}

/** The enrollments that took one path of a fork, over every version that had it */
model AutomationPathFigures {
  /** The fork's Step id, the same in every version */
  stepId: StepId;

  /** The path's id, the same in every version */
  pathId: StepId;

  /** Enrollments that took the path */
  enrollments: int32;

  /** Enrollments among them that ended because their goal event was recorded */
  goalReached: int32;
}

/** The messages of one send Step, over every version that had it */
model AutomationStepFigures {
  /** The Step's id, the same in every version */
  stepId: StepId;

  /** Messages the provider accepted */
  sent: int32;

  /** Messages skipped: the contact had no address on the step's Channel, had opted out of it, or
   * was suppressed. The enrollment moved on to the next step. */
  skipped: int32;
}

/** The messages one version's send Step sent */
model AutomationVersionStepFigures {
  /** The version's number */
  version: int32;

  /** The Step's id */
  stepId: StepId;

  /** Messages the provider accepted */
  sent: int32;

  /** Messages skipped, as in AutomationStepFigures */
  skipped: int32;
}
