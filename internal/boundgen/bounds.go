package boundgen

const (
	collect   = "openapi/collect.openapi.json"
	site      = "openapi/site.openapi.json"
	external  = "openapi/external.openapi.json"
	service   = "openapi/service.openapi.json"
	staff     = "openapi/staff.openapi.json"
	staffee   = "openapi/staffee.openapi.json"
	telemetry = "openapi/telemetry.openapi.json"
)

// The names the lists repeat: a bound's, an owner's, a parameter's and a schema's.
const (
	contactCity     = "ContactCity"
	city            = "city"
	contactOwner    = "Contact"
	limit           = "limit"
	telemetryReport = "TelemetryReport"
)

// Bounds are the contract's bounds on the input of a domain write, in the order they are written.
// A bound added here is checked by each of its owners: the bound-ownership test refuses one that an
// owner's code does not use. An entry that repeats a Name is another place of that bound.
var Bounds = []Bound{
	{
		Name: "Name", Doc: "is what a person calls a named row (a Tag, a Segment, a Template, ...), trimmed.",
		Spec: site, Schema: "Name", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "UserName", Doc: "is what a User is called, trimmed; a Sign-in provider's name has the same length.",
		Spec: site, Schema: "UserName", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "EmailAddress", Doc: "is a stored email address.",
		Spec: site, Schema: "EmailAddress", Owners: []string{"internal/kernel/validation"},
	},
	{
		Name: "HoldReasonCode", Doc: "is the machine code of a Hold's reason.",
		Spec: service, Schema: "HoldInput", Property: "reasonCode", Owners: []string{"Hold"},
	},
	{
		Name: "HoldMessage", Doc: "is the message a Hold shows the Workspace's Owner.",
		Spec: service, Schema: "HoldInput", Property: "message", Owners: []string{"Hold"},
	},
	{
		Name: "HoldActionURL", Doc: "is where an Owner or Admin can resolve a Hold.",
		Spec: service, Schema: "HoldInput", Property: "actionUrl", Owners: []string{"Hold"},
	},
	{
		Name: "MaxPerDay", Doc: "is a rolling 24-hour limit: a Provider limit's daily value or a Send cap.",
		Spec: site, Schema: "MaxPerDay", Owners: []string{"Workspace", "DefaultSendCap", "Integration"},
	},
	{
		Name: "MaxPerSecond", Doc: "is a Provider limit's value per second.",
		Spec: site, Schema: "MaxPerSecond", Owners: []string{"Integration"},
	},
	{
		Name: "SesConfigurationSet", Doc: "is the name of the SES configuration set an SES Integration's sends name.",
		Spec: site, Schema: "SesConfigurationSetName", Owners: []string{"internal/domain/sending/integrations"},
	},
	{
		Name: "AttributionWindowDays", Doc: "is how many days a Workspace's Attribution window lasts.",
		Spec: site, Schema: "AttributionWindowDays", Owners: []string{"Workspace"},
	},
	{
		Name: "RetentionDays", Doc: "is how many days a Workspace's Audit log is kept.",
		Spec: site, Schema: "RetentionDays", Owners: []string{"Workspace"},
	},
	{
		Name: "HTTPCallHeaders", Doc: "are the headers of an http Action or step.",
		Spec: site, Schema: "RuleActionInput", Property: "headers", Owners: []string{"internal/infra/httpcall"},
	},
	{Name: "HTTPCallHeaders", Spec: site, Schema: "AutomationStepInput", Property: "headers"},
	{
		Name: contactCity, Doc: "is a Contact's city.",
		Spec: external, Schema: "UpsertContactInput", Property: city, Owners: []string{contactOwner},
	},
	{Name: contactCity, Spec: external, Schema: "CreateContactInput", Property: city},
	{Name: contactCity, Spec: external, Schema: "UpdateContactInput", Property: city},
	{Name: contactCity, Spec: site, Schema: "SiteCreateContactInput", Property: city},
	{Name: contactCity, Spec: site, Schema: "SiteUpdateContactInput", Property: city},
	{
		Name: "LocaleTag", Doc: "is a BCP 47 language tag, a Contact's locale.",
		Spec: site, Schema: "LocaleTag", Owners: []string{contactOwner},
	},
	{
		Name: "CountryCode", Doc: "is an ISO 3166-1 alpha-2 country code, a Contact's country.",
		Spec: site, Schema: "CountryCode", Owners: []string{contactOwner},
	},
	{
		Name: "ImageURL", Doc: "is the https URL of an image, a Contact's avatar.",
		Spec: site, Schema: "ImageUrl", Owners: []string{contactOwner},
	},
	{
		Name: "BrandKitLogoURL", Doc: "is the URL of a Brand kit's logo.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "logoUrl", Owners: []string{"BrandKit"},
	},
	{
		Name: "BrandKitButtonRadius", Doc: "is the corner radius of a Brand kit's buttons, in pixels.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "buttonRadius", Owners: []string{"BrandKit"},
	},
	{
		Name: "BrandKitFooterText", Doc: "is the footer text of a Brand kit.",
		Spec: site, Schema: "SiteSaveBrandKitInput", Property: "footerText", Owners: []string{"BrandKit"},
	},
	{
		Name: "SignupSurveyOther", Doc: "is the text of a Sign-up survey answer's `other` choice.",
		Spec: site, Schema: "SiteSignupSurveyInput", Property: "roleOther", Owners: []string{"SignupSurveyResponse"},
	},
	{Name: "SignupSurveyOther", Spec: site, Schema: "SiteSignupSurveyInput", Property: "companySizeOther"},
	{Name: "SignupSurveyOther", Spec: site, Schema: "SiteSignupSurveyInput", Property: "mainGoalOther"},
	{Name: "SignupSurveyOther", Spec: site, Schema: "SiteSignupSurveyInput", Property: "heardFromOther"},
	{
		Name: "CollectEvents", Doc: "are the Events one request of the tracker sends: its whole queue at most.",
		Spec: collect, Schema: "CollectEventsInput", Property: "events", Owners: []string{"packages/analytics/src"},
	},
	{
		Name: "SignupItemKey", Doc: "is the key a Sign-up item is answered under.",
		Spec: site, Schema: "SignupItemKey", Owners: []string{"SignupItem"},
	},
	{
		Name: "SignupItemTitle", Doc: "is the title of a Sign-up item.",
		Spec: site, Schema: "SignupItemTitle", Owners: []string{"SignupItem"},
	},
	{
		Name: "LegalDocumentLabel", Doc: "is the label of a published version of a Legal document.",
		Spec: site, Schema: "LegalDocumentLabel", Owners: []string{"LegalDocumentVersion"},
	},
	{
		Name: "LegalDocumentMarkdown", Doc: "is the text of a Legal document, in Markdown.",
		Spec: site, Schema: "LegalDocumentMarkdown", Owners: []string{"LegalDocumentDraft"},
	},
	{
		Name: "ConsentWording", Doc: "is the wording a person consented to.",
		Spec: site, Schema: "ConsentEvidence", Property: "wording", Owners: []string{"Confirmation"},
	},
	{Name: "ConsentWording", Spec: external, Schema: "CreateConfirmationInput", Property: "wording"},
	{
		Name: "ConsentSource", Doc: "is where a person consented.",
		Spec: site, Schema: "ConsentEvidence", Property: "source", Owners: []string{"Confirmation"},
	},
	{Name: "ConsentSource", Spec: external, Schema: "CreateConfirmationInput", Property: "source"},
	{
		Name: "ConsentBrand", Doc: "is the brand a person consented to hear from.",
		Spec: site, Schema: "ConsentEvidence", Property: "brand", Owners: []string{"Confirmation"},
	},
	{Name: "ConsentBrand", Spec: external, Schema: "CreateConfirmationInput", Property: "brand"},
	{
		Name: "TrackingDomainHost", Doc: "is the host a Workspace serves its links and tracker on, as typed.",
		Spec: site, Schema: "SetTrackingDomainInput", Property: "host", Owners: []string{"TrackingDomain"},
	},
	{
		Name: "Page", Doc: "is the 1-based number of a page of a list.",
		Spec: site, Parameter: "PageQuery.page", Owners: []string{"internal/kernel/pagination"},
	},
	{
		Name: "PageSize", Doc: "is how many items a page of a list holds.",
		Spec: site, Parameter: "PageQuery.pageSize", Owners: []string{"internal/kernel/pagination"},
	},
	{
		Name: "TelegramButtons", Doc: "are the buttons of a Telegram message.",
		Spec: site, Schema: "TelegramBroadcastContentInput", Property: "buttons",
		Owners: []string{"internal/kernel/telegramrender"},
	},
	{Name: "TelegramButtons", Spec: site, Schema: "AutomationStepInput", Property: "buttons"},
	{
		Name: "TelegramUnsubscribeButton", Doc: "is the label of a Telegram message's unsubscribe button.",
		Spec: site, Schema: "TelegramBroadcastContentInput", Property: "unsubscribeButton",
		Owners: []string{"internal/kernel/telegramrender"},
	},
	{Name: "TelegramUnsubscribeButton", Spec: site, Schema: "AutomationStepInput", Property: "unsubscribeButton"},
	{
		Name: "VisitorID", Doc: "is the tracker's id of a Visitor, the device an Event, an Identify or a start link names.",
		Spec: collect, Schema: "CollectEventInput", Property: "visitorId", Owners: []string{"Visitor"},
	},
	{Name: "VisitorID", Spec: collect, Schema: "CollectIdentifyInput", Property: "visitorId"},
	{Name: "VisitorID", Spec: collect, Schema: "CollectTelegramStartLinkInput", Property: "visitorId"},
	{
		Name: "CollectEventAction", Doc: "is the action of an Event the tracker collects.",
		Spec: collect, Schema: "CollectEventInput", Property: "action", Owners: []string{"internal/domain/contacts/visitors"},
	},
	{
		Name: "ClientEventID", Doc: "is a client's own id of an Event, which makes a retry of it a no-op.",
		Spec: collect, Schema: "CollectEventInput", Property: "clientEventId",
		Owners: []string{"internal/domain/contacts/visitors", "internal/domain/contacts/eventrecording"},
	},
	{Name: "ClientEventID", Spec: external, Schema: "EventInput", Property: "clientEventId"},
	{
		Name: "EventBatch", Doc: "are the Events one request of the external API records.",
		Spec: external, Schema: "RecordEventsBatchInput", Property: "events",
		Owners: []string{"internal/domain/contacts/eventrecording"},
	},
	{
		Name: "ContactBatch", Doc: "are the Contacts one request of the external API upserts.",
		Spec: external, Schema: "UpsertContactsInput", Property: "contacts", Owners: []string{contactOwner},
	},
	{
		Name: "IdentifyPhone", Doc: "is the phone an Identify of the tracker names its Contact by.",
		Spec: collect, Schema: "CollectIdentifyInput", Property: "phone",
		Owners: []string{"internal/usecase/visitoridentification"},
	},
	{
		Name: "IdentifySubjectID", Doc: "is the subject id an Identify of the tracker names its Contact by.",
		Spec: collect, Schema: "CollectIdentifyInput", Property: "subjectId",
		Owners: []string{"internal/usecase/visitoridentification"},
	},
	{
		Name: "IdentifyTimeZone", Doc: "is the time zone the browser of an Identify reports.",
		Spec: collect, Schema: "CollectIdentifyContext", Property: "timeZone",
		Owners: []string{"internal/usecase/visitoridentification"},
	},
	{
		Name: "IdentifyLocale", Doc: "is the language the browser of an Identify reports.",
		Spec: collect, Schema: "CollectIdentifyContext", Property: "locale",
		Owners: []string{"internal/usecase/visitoridentification"},
	},
	{
		Name: "ConsentDestination", Doc: "is the address or phone a Consent fact (a Confirmation, a Suppression, an Unsubscribe) is about.",
		Spec: external, Schema: "CreateConfirmationInput", Property: "destination",
		Owners: []string{"Confirmation", "Suppression", "Unsubscribe"},
	},
	{Name: "ConsentDestination", Spec: external, Schema: "CreateSuppressionInput", Property: "destination"},
	{Name: "ConsentDestination", Spec: external, Schema: "CreateUnsubscribeInput", Property: "destination"},
	{
		Name: "RuleTriggerEvents", Doc: "are the Event actions that fire a Rule.",
		Spec: site, Schema: "CreateRuleInput", Property: "triggerEvents", Owners: []string{"Rule"},
	},
	{Name: "RuleTriggerEvents", Spec: site, Schema: "UpdateRuleInput", Property: "triggerEvents"},
	{
		Name: "ConditionWithinDays", Doc: "is how many days back a condition leaf looks: an Event count's window or a date's.",
		Spec: site, Schema: "ConditionEventLeafInput", Property: "withinDays",
		Owners: []string{"internal/domain/contacts/condition"},
	},
	{Name: "ConditionWithinDays", Spec: site, Schema: "ConditionDateWithinLeafInput", Property: "withinDays"},
	{
		Name: "ConditionEventTimes", Doc: "is how many Events of an action a condition leaf counts.",
		Spec: site, Schema: "ConditionEventLeafInput", Property: "times",
		Owners: []string{"internal/domain/contacts/condition"},
	},
	{
		Name: "CurrencyCode", Doc: "is an ISO 4217 currency code, a Workspace's reporting currency.",
		Spec: site, Schema: "CurrencyCode", Owners: []string{"Workspace"},
	},
	{
		Name: "RegionalFormat", Doc: "is the BCP 47 tag of how the app shows a User dates, times and numbers.",
		Spec: site, Schema: "RegionalFormat", Owners: []string{"User"},
	},
	{
		Name: "SuspensionReason", Doc: "is why a Workspace is suspended, which its owners are told.",
		Spec: staffee, Schema: "StaffSuspendInput", Property: "reason",
		Owners: []string{"internal/domain/identity/suspension"},
	},
}

// TransportOnly are the bounds of what a client sends that no write meets, each with the reason.
var TransportOnly = []TransportBound{
	{Spec: site, Schema: "EntityId", Doc: "is the form of an entity's id in a path, which ogen parses before a module sees the number."},
	{Spec: site, Schema: "WorkspaceId", Doc: "is the form of a Workspace's id in a path, which ogen parses before a module sees the number."},
	{Spec: site, Schema: "Revision", Doc: "is the revision a write expects, a number the module compares and never stores as sent."},
	{Spec: external, Parameter: "ExportPageQuery.limit", Doc: "is the size of a page an export reads."},
	{Spec: external, Operation: "AuditEntries_list", Parameter: limit, Doc: "is the size of a page of the Audit log a read returns."},
	{Spec: external, Operation: "SendingDomainRates_list", Parameter: "windowDays", Doc: "is how many days of a Sending domain's rates a read returns."},
	{Spec: site, Operation: "SiteAudit_list", Parameter: limit, Doc: "is the size of a page of the Audit log a read returns."},
	{Spec: site, Operation: "SiteContacts_lookup", Parameter: "email", Doc: "is the address a read looks a Contact up by."},
	{Spec: site, Operation: "SiteContacts_erase", Parameter: "email", Doc: "is the address an erasure finds its Contact by; it writes no address."},
	{Spec: service, Parameter: "UsagePeriodQuery", Doc: "is the period a read of the usage covers."},
	{Spec: service, Operation: "ServiceTrackingHostFeed_list", Parameter: limit, Doc: "is the size of a page of a /changes read."},
	{Spec: service, Operation: "ServiceUserFeed_list", Parameter: limit, Doc: "is the size of a page of a /changes read."},
	{Spec: service, Operation: "ServiceWorkspaceFeed_list", Parameter: limit, Doc: "is the size of a page of a /changes read."},
	{Spec: staff, Operation: "StaffInstance_listAuditEntries", Parameter: limit, Doc: "is the size of a page of the Audit log a read returns."},
	{Spec: staffee, Operation: "StaffWorkspaceAudit_list", Parameter: limit, Doc: "is the size of a page of the Audit log a read returns."},
	{
		Spec: collect, Schema: "CollectIdentifyInput", Property: "identityToken",
		Doc: "is an Identity token, which the module verifies and never stores: its length caps only what the transport reads.",
	},
	{
		Spec: collect, Schema: "CollectTelegramStartLinkInput", Property: "identityToken",
		Doc: "is an Identity token, which the module verifies and never stores: its length caps only what the transport reads.",
	},
	{Spec: telemetry, Schema: telemetryReport, Property: "appVersion", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "arch", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "instanceAgeDays", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "instanceId", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "integrationKinds", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "os", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "postgresMajor", Doc: telemetryDoc},
	{Spec: telemetry, Schema: telemetryReport, Property: "schemaVersion", Doc: telemetryDoc},
}

// telemetryDoc is why only the transport checks a field of the Telemetry report: the Instance sends
// the report and never receives one, and the report's tests hold what it sends to the published
// schema (internal/telemetryschema).
const telemetryDoc = "is a field of the Telemetry report, which the Instance sends and never receives."
