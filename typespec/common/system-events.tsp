// The actions of the System events: /site and /api show and filter by them next to a customer's own
// actions, and /collect refuses a customer's action in their namespaces.

namespace Sphericon;

/** The action of a System event: an Event sphericon itself records, never a customer. An Event's
 * `action`, an Automation's trigger and a condition carry one of these or a customer's own action,
 * which may not take a System event's namespace; a webhook's event types may also name the Audit
 * log's `audit.entry`. */
enum SystemEventAction {
  /** A Contact was created */
  contactCreated: "contact.created",

  /** A Contact was erased */
  contactErased: "contact.erased",

  /** An email was handed to the provider */
  emailSent: "email.sent",

  /** An email reached the recipient's server */
  emailDelivered: "email.delivered",

  /** The recipient's server deferred an email */
  emailDelayed: "email.delayed",

  /** The provider dropped an email without sending it */
  emailDropped: "email.dropped",

  /** The Contact opened an email */
  emailOpened: "email.opened",

  /** The Contact clicked a link of an email */
  emailClicked: "email.clicked",

  /** The Contact unsubscribed from email */
  emailUnsubscribed: "email.unsubscribed",

  /** An email bounced, hard or soft */
  emailBounced: "email.bounced",

  /** The Contact marked an email as spam */
  emailComplained: "email.complained",

  /** The Contact confirmed its email address for marketing */
  marketingConfirmed: "marketing.confirmed",

  /** A Tag was applied to the Contact */
  contactTagged: "contact.tagged",

  /** A Tag was removed from the Contact */
  contactUntagged: "contact.untagged",

  /** A Custom field of the Contact changed */
  contactFieldChanged: "contact.field_changed",

  /** The Contact was enrolled in an Automation */
  automationEnrolled: "automation.enrolled",

  /** The Contact completed an Automation */
  automationCompleted: "automation.completed",

  /** The Contact reached an Automation's goal */
  automationGoalReached: "automation.goal_reached",

  /** The Contact left an Automation on an exit event */
  automationExited: "automation.exited",

  /** A Telegram chat was bound to the Contact */
  telegramBound: "telegram.bound",

  /** A Telegram chat's binding moved away from the Contact */
  telegramUnbound: "telegram.unbound",

  /** The Contact sent the Telegram bot a message */
  telegramReceived: "telegram.received",

  /** Telegram accepted a message for the Contact */
  telegramSent: "telegram.sent",

  /** The Contact clicked a link of a Telegram message */
  telegramClicked: "telegram.clicked",

  /** The Contact stopped the Workspace's Telegram marketing */
  telegramUnsubscribed: "telegram.unsubscribed",

  /** The Contact sent a text to an SMS Integration's sender */
  smsReceived: "sms.received",

  /** The SMS provider accepted a text for the Contact */
  smsSent: "sms.sent",

  /** A text reached the Contact's phone */
  smsDelivered: "sms.delivered",

  /** A text did not reach the Contact's phone */
  smsUndelivered: "sms.undelivered",

  /** The Contact confirmed its phone number for SMS marketing */
  smsConfirmed: "sms.confirmed",

  /** The Contact clicked a link of a text */
  smsClicked: "sms.clicked",

  /** The Contact's number opted out of the Workspace's SMS marketing */
  smsUnsubscribed: "sms.unsubscribed",
}
