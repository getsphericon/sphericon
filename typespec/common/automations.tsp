// The Automation models /site and /api show and take the same way: its steps, versions, write
// bodies, HTTP step dry run and report.

import "./engagement.tsp";
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

/** One Step of an automation, by `type` */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union AutomationStep {
  email: EmailStep,
  telegram: TelegramStep,
  sms: SmsStep,
  wait: WaitStep,
  apply_tag: ApplyTagStep,
  remove_tag: RemoveTagStep,
  http: HttpStep,
}

/** One Step of an automation, by `type`, as a request writes it. A field of another kind is refused. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
union AutomationStepInput {
  email: EmailStepInput,
  telegram: TelegramStepInput,
  sms: SmsStepInput,
  wait: WaitStepInput,
  apply_tag: ApplyTagStepInput,
  remove_tag: RemoveTagStepInput,
  http: HttpStepInput,
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

/** One version of an automation's definition: what publishing makes it do (ADR 0065) */
model AutomationVersion {
  /** 1 for the first version, plus one per version after it */
  number: int32;

  /** Event action that enrolls a contact (e.g. "contact.created", "email.opened") */
  triggerEvent: string;

  /** Event actions that end an active enrollment early, as exited (e.g. "plan_downgraded"). Only
   * an Event that occurred once the enrollment began counts; machine opens and clicks never do. */
  exitEvents: string[];

  /** Event action that ends an active enrollment as goal_reached (e.g. "payment_succeeded"); null
   * when there is none */
  goalEvent: string | null;

  /** The version's Steps: its root path */
  steps: AutomationPath;
}

/** Request body for creating an automation (always created inactive) */
model CreateAutomationInput {
  name: Name;
  triggerEvent: string;

  /** Event actions that end an active enrollment early; none of them may be the trigger. Each
   * follows the trigger's rule: any customer action, and of the email events only opened, clicked
   * and unsubscribed. */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; it may be neither the trigger nor
   * an exit event. Null or omitted is none. */
  goalEvent?: string | null;

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
  triggerEvent?: string;

  /** Event actions that end an active enrollment early; replaces the list, omitted keeps it */
  exitEvents?: string[];

  /** Event action that ends an active enrollment as goal_reached; null removes it, omitted keeps it */
  goalEvent?: string | null;

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

  /** Mean time in whole seconds from the beginning to the end of the enrollments that ended in
   * the period, whatever their outcome; null when none ended */
  averageDurationSeconds: int64 | null;

  /** The orders credited to the automation that were completed in the period, and the net change
   * of its revenue in the period: every Movement of a credited Order counts on its own day (ADR 0064) */
  revenue: MessageRevenue;
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

  /** The send steps' messages by step position, in step order: one entry per position any of the
   * automation's messages was sent from, across its versions */
  steps: AutomationStepFigures[];
}

/** The messages of the send step at one position */
model AutomationStepFigures {
  /** The step's position, 0 for the first step */
  step: int32;

  /** Messages the provider accepted */
  sent: int32;

  /** Messages skipped: the contact had no address on the step's Channel, had opted out of it, or
   * was suppressed. The enrollment moved on to the next step. */
  skipped: int32;
}
