// Package resources holds goverter-generated mappers from ent entities to site
// API resources (the ogen *Resource DTOs), plus the custom conversions goverter
// cannot infer on its own. The generated implementation (ConverterImpl) lives in
// converter_gen.go; consumers instantiate it (see internal/api/site).
package resources

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/integrations"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/linkclicks"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

// Converter maps ent entities to site API resources. goverter generates the
// implementation; named string/time casts are inferred, the extends below cover
// the conversions it can't (ids, optionals, the event property bag).
//
// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum no
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optNilString
// goverter:extend optNilContactSource
// goverter:extend omitEmpty
// goverter:extend optNilEmailAddress
// goverter:extend optNilEntityID
// goverter:extend optNilTimeZone
// goverter:extend nilCurrencyCode
// goverter:extend nilTimeZone
// goverter:extend attributionWindowDays
// goverter:extend workspaceTimezone
// goverter:extend optNilTimestamp
// goverter:extend contactCustomFields
// goverter:extend optNilInt32
// goverter:extend exportJSON
// goverter:extend eventProperties
// goverter:extend templateDoc
// goverter:extend broadcastDoc
// goverter:extend automationSteps
// goverter:extend segmentCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend emailVerified
// goverter:extend transactionalChannel
// goverter:extend transactionalStatus
// goverter:extend transactionalError
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend providerLimitWarning
// goverter:extend providerLimitSource
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optMembershipSource
// goverter:extend optTrackingReason
// goverter:extend timestampOf
// goverter:extend tagNames
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend optConsentDecision
type Converter interface {
	// The contact is read with its tags loaded (`WithTags`).
	// goverter:map Edges.Tags Tags
	ContactToResource(source *ent.Contact) siteapi.SiteContactResource
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) siteapi.SiteSegmentResource
	TokenToResource(source *ent.ApiToken) siteapi.SiteApiTokenResource
	// A Connection is an API token read with its Membership and that Membership's User loaded
	// (apitokens.Connections); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	ConnectionToResource(source *ent.ApiToken) siteapi.SiteConnectionResource
	// A Personal token is an API token read with its Membership and that Membership's User loaded
	// (apitokens.PersonalTokens, MintPersonal); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	PersonalTokenToResource(source *ent.ApiToken) siteapi.SitePersonalTokenResource

	// Verification is derived: a non-nil email_verified_at means verified.
	// The Sign-up survey is due only while the Instance still asks it, which the mapper cannot
	// see: the handler sets it from the marker and the Instance switch.
	// goverter:map EmailVerifiedAt EmailVerified | emailVerified
	// goverter:ignore SignupSurveyDue
	// The pending acceptances are read from the Consent records and the Sign-up items in effect,
	// which the mapper cannot see: the handler attaches them (SignupItemsToSite).
	// goverter:ignore AcceptancesPending
	UserToResource(source *ent.User) *siteapi.SiteUserResource
	// The provider is shown by its display name, which the Instance's registry holds and the
	// mapper cannot see: the handler sets it.
	// goverter:ignore Provider
	PendingSignupToResource(source *ent.PendingSignup) *siteapi.SitePendingSignup
	// The grace end, the role and its Permissions belong to the authenticated User's Membership,
	// which the mapper cannot see: the handler attaches them.
	// goverter:ignore SecondFactorGraceEndsAt
	// goverter:ignore Role
	// goverter:ignore Permissions
	// goverter:ignore AccountLinkAvailable
	// The active holds are read through the Workspace's scope, which the mapper cannot
	// see: the handler attaches them.
	// goverter:ignore Holds
	// The support contact is instance configuration, attached by the handler.
	// goverter:ignore SupportEmail
	// The MCP origin is instance configuration, attached by the handler.
	// goverter:ignore McpUrl
	// goverter:ignore TrackerUrl
	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	WorkspaceToResource(source *ent.Workspace) siteapi.SiteWorkspaceResource
	// The revenue settings are two columns of the Workspace (ADR 0064).
	RevenueSettingsToResource(source *ent.Workspace) siteapi.SiteRevenueSettingsResource
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:map Currency ReportingCurrency
	RevenueToResource(source *analytics.Revenue) siteapi.SiteAnalyticsRevenue
	EventToResource(source *ent.Event) siteapi.SiteEventResource
	LinkClicksToResource(source []linkclicks.Clicks) []siteapi.SiteLinkClicks

	// The delivery figures are derived from the Broadcast's recipients and their Outbound
	// messages, which the mapper cannot see: the handler attaches them (BroadcastStats).
	// goverter:ignore Stats
	// Progress is derived from the recipients, which the mapper cannot see: the
	// handler attaches it.
	// goverter:ignore Progress
	// Warnings are the lint of the content just saved (ADR 0040): the handler attaches them
	// to the response of a save.
	// goverter:ignore Warnings
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	BroadcastToResource(source *ent.Broadcast) siteapi.SiteBroadcastResource

	// goverter:map Processed ProcessedCount
	// goverter:map Remaining RemainingCount
	// goverter:map EstimatedCompletion EstimatedCompletionAt
	BroadcastProgressToResource(source broadcasts.Progress) siteapi.SiteBroadcastProgress

	// goverter:ignore Warnings
	EmailTemplateToResource(source *ent.EmailTemplate) siteapi.SiteEmailTemplateResource

	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// The live version and the draft are the edges the automations module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	// The revenue is summed from Order Movements, which the mapper cannot see: the handler
	// attaches it.
	// goverter:ignore Revenue
	AutomationToResource(source *ent.Automation) siteapi.SiteAutomationResource
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map . Steps | automationSteps
	// goverter:map IntegrationID IntegrationId
	AutomationVersionToResource(source *ent.AutomationVersion) siteapi.SiteAutomationVersion

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Provider limit from providerlimit.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore ProviderLimit
	// goverter:map DiscardedAt ArchivedAt
	IntegrationToResource(source *ent.Integration) siteapi.SiteIntegrationResource
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) siteapi.SiteSmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) siteapi.SiteSesConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) siteapi.SiteProviderLimitStatus
	PasskeyToResource(source *ent.Passkey) siteapi.SitePasskey
	SuppressionToResource(source *ent.Suppression) siteapi.SiteSuppressionResource
	CustomFieldToResource(source *ent.CustomField) siteapi.SiteCustomFieldResource
	TagToResource(source *ent.Tag) siteapi.SiteTagResource

	// The transactional send history is the Outbound messages of kind transactional
	// (ADR 0015). A message skipped by Send-eligibility is shown as "suppressed" (the
	// only reason a transactional send is skipped); the reason column is the error
	// text only for a failed send.
	// goverter:map . Status | transactionalStatus
	// goverter:map . Error | transactionalError
	// goverter:map TemplateID TemplateId
	TransactionalMessageToResource(source *ent.OutboundMessage) siteapi.SiteTransactionalEmailResource

	// A Captured message names its From as the contract does, and the nullable parts as null.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToResource(source *ent.CapturedMessage) siteapi.SiteCapturedMessageResource
	// The list leaves the content out.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToSummary(source *ent.CapturedMessage) siteapi.SiteCapturedMessageSummary

	// The member's identity comes from the loaded User edge.
	// goverter:map Edges.User.Email Email
	// goverter:map Edges.User.Name Name
	MembershipToResource(source *ent.Membership) siteapi.SiteMembershipResource

	// goverter:map . InvitedByEmail | inviterEmail
	InvitationToResource(source *ent.Invitation) siteapi.SiteInvitationResource

	// The contract names the discarded state "archived" (ADR 0049). The live version and the draft
	// are the edges the rules module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) siteapi.SiteRuleResource

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) siteapi.SiteRuleVersion

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) siteapi.SiteRuleFiringResource

	RuleDryRunToResource(source rules.DryRun) siteapi.SiteRuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	EnrollmentToResource(source *ent.AutomationRun) siteapi.SiteEnrollmentResource

	// The deletion window: the date the Workspace's data will be erased.
	// goverter:map PublicID ID
	// goverter:map DiscardedAt EraseAt | eraseAt
	// Whether the caller is its only Owner comes from the Memberships, which the mapper
	// cannot see: the handler attaches it.
	// goverter:ignore OnlyOwner
	DeletedWorkspaceToResource(source *ent.Workspace) siteapi.SiteDeletedWorkspaceResource

	// goverter:map CreatedAt Since
	HoldToResource(source *ent.Hold) siteapi.SiteWorkspaceHold

	// goverter:map . Record | joinDomainRecord
	// goverter:map . Verified | joinDomainVerified
	JoinDomainToResource(source *ent.JoinDomain) siteapi.SiteJoinDomainResource

	// The DNS records are derived by the sendingdomains module, which the mapper cannot
	// see: the handler attaches them. The private key is never exposed.
	// goverter:ignore DkimRecord
	// goverter:ignore SpfRecord
	// goverter:ignore DmarcRecord
	// goverter:ignore TrackingRecord
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) siteapi.SiteSendingDomainResource
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) siteapi.SiteDnsRecord
	// goverter:map . Type | trackingRecordType
	// goverter:map CheckedAt LastCheckedAt
	TrackingRecordToResource(source sendingdomains.TrackingRecord) siteapi.SiteTrackingRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkit.Kit) siteapi.SiteBrandKitResource

	// goverter:map . Actor | auditActor
	// goverter:map . Target | auditTarget
	// goverter:map Diff Diff | auditDiff
	AuditEntryToResource(source auditapi.View) siteapi.SiteAuditEntryResource

	// The signing secret is decrypted by the webhooks module, which the mapper cannot see:
	// the handler attaches it. The failure state is derived from the failure columns.
	// goverter:ignore Secret
	// goverter:map EventTypes EventTypes | eventTypes
	// goverter:map . Failure | webhookFailure
	// goverter:map . Enabled | webhookEnabled
	WebhookEndpointToResource(source *ent.WebhookEndpoint) siteapi.SiteWebhookEndpointResource

	// The Data export members (ADR 0021): the contract's typed projection of the
	// stored rows, so the ent JSON tags are never the public format.
	ContactToExport(source *ent.Contact) siteapi.ContactExportContact
	TagToExport(source *ent.Tag) siteapi.ContactExportTag
	VisitorToExport(source *ent.Visitor) siteapi.ContactExportVisitor
	EventToExport(source *ent.Event) siteapi.ContactExportEvent
	UnsubscribeToExport(source *ent.Unsubscribe) siteapi.ContactExportUnsubscribe
	SuppressionToExport(source *ent.Suppression) siteapi.ContactExportSuppression
	ConfirmationToExport(source *ent.Confirmation) siteapi.ContactExportConfirmation
	OutboundMessageToExport(source *ent.OutboundMessage) siteapi.ContactExportOutboundMessage
	BroadcastRecipientToExport(source *ent.BroadcastRecipient) siteapi.ContactExportBroadcastRecipient

	// The Sign-up items in effect and their Legal documents (ADR 0062): the contract calls the
	// current version's label the item's version.
	// goverter:map Label Version
	SignupItemToSite(source signupitems.Document) siteapi.SiteSignupItem
	SignupItemsToSite(source []signupitems.Document) []siteapi.SiteSignupItem
	// goverter:map Label Version
	LegalDocumentToSite(source signupitems.Document) siteapi.SiteLegalDocument
	// The optional consents of account settings with the User's current answer (ADR 0062).
	OptionalConsentToSite(source consentrecords.OptionalConsent) siteapi.SiteOptionalConsent
	OptionalConsentsToSite(source []consentrecords.OptionalConsent) []siteapi.SiteOptionalConsent
	// The banner of a newer release, the one part of the Instance a Workspace's owner sees (ADR 0032).
	InstanceReleaseToResource(source instance.Release) siteapi.SiteInstanceRelease
}

// ExportMapper adapts the Converter to contactexport.Mapper.
type ExportMapper struct{ Converter }

func (m ExportMapper) Contact(r *ent.Contact) json.Marshaler { return new(m.ContactToExport(r)) }
func (m ExportMapper) Tag(r *ent.Tag) json.Marshaler         { return new(m.TagToExport(r)) }
func (m ExportMapper) Visitor(r *ent.Visitor) json.Marshaler { return new(m.VisitorToExport(r)) }
func (m ExportMapper) Event(r *ent.Event) json.Marshaler     { return new(m.EventToExport(r)) }
func (m ExportMapper) Unsubscribe(r *ent.Unsubscribe) json.Marshaler {
	return new(m.UnsubscribeToExport(r))
}
func (m ExportMapper) Suppression(r *ent.Suppression) json.Marshaler {
	return new(m.SuppressionToExport(r))
}
func (m ExportMapper) Confirmation(r *ent.Confirmation) json.Marshaler {
	return new(m.ConfirmationToExport(r))
}
func (m ExportMapper) OutboundMessage(r *ent.OutboundMessage) json.Marshaler {
	return new(m.OutboundMessageToExport(r))
}
func (m ExportMapper) BroadcastRecipient(r *ent.BroadcastRecipient) json.Marshaler {
	return new(m.BroadcastRecipientToExport(r))
}

// providerLimitWarning casts: providerlimit's warnings are the contract's enum values.
func providerLimitWarning(w providerlimit.Warning) siteapi.SiteProviderLimitWarning {
	return siteapi.SiteProviderLimitWarning(w)
}

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

// nilString renders an optional value: null when there is none.
func nilString(v *string) siteapi.NilString {
	if v == nil {
		return siteapi.NilString{Null: true}
	}
	return siteapi.NewNilString(*v)
}

// nilEntityID renders an optional reference: null when there is none.
func nilEntityID(v *int64) siteapi.NilEntityId {
	if v == nil {
		return siteapi.NilEntityId{Null: true}
	}
	return siteapi.NewNilEntityId(siteapi.EntityId(*v))
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) siteapi.NilInt32 {
	if v == nil {
		return siteapi.NilInt32{Null: true}
	}
	return siteapi.NewNilInt32(narrow.Int32(*v))
}

// providerLimitSource is null exactly when the window has no limit (its source is empty).
func providerLimitSource(v providerlimit.Source) siteapi.NilSiteProviderLimitSource {
	if v == "" {
		return siteapi.NilSiteProviderLimitSource{Null: true}
	}
	return siteapi.NewNilSiteProviderLimitSource(siteapi.SiteProviderLimitSource(v))
}

func optNilInt32(v *int) siteapi.OptNilInt32 {
	if v == nil {
		return siteapi.OptNilInt32{}
	}
	return siteapi.NewOptNilInt32(narrow.Int32(*v))
}

// exportJSON renders a stored JSON object (custom fields, event properties).
func exportJSON(m map[string]any) siteapi.OptNilContactExportJson {
	if len(m) == 0 {
		return siteapi.OptNilContactExportJson{}
	}
	out := make(siteapi.ContactExportJson, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilContactExportJson(out)
}

// emailVerified derives the verified flag from the nullable timestamp.
func emailVerified(t *time.Time) bool {
	return t != nil
}

func timestamp(t time.Time) siteapi.Timestamp {
	return siteapi.Timestamp(t)
}

// transactionalChannel maps the Outbound message channel onto the transactional
// send channel enum (email is the only channel built).
func transactionalChannel(outboundmessage.Channel) siteapi.SiteTransactionalEmailChannel {
	return siteapi.SiteTransactionalEmailChannelEmail
}

// transactionalStatus maps an Outbound message's status onto the transactional
// send status enum.
func transactionalStatus(m ent.OutboundMessage) siteapi.SiteTransactionalEmailStatus {
	switch m.Status {
	case outboundmessage.StatusSent:
		return siteapi.SiteTransactionalEmailStatusSent
	case outboundmessage.StatusSkipped:
		return siteapi.SiteTransactionalEmailStatusSuppressed
	case outboundmessage.StatusFailed:
		return siteapi.SiteTransactionalEmailStatusFailed
	default:
		return siteapi.SiteTransactionalEmailStatusPending
	}
}

// transactionalError exposes the failure text only for a failed send.
func transactionalError(m ent.OutboundMessage) siteapi.OptNilString {
	if m.Status != outboundmessage.StatusFailed || m.Reason == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*m.Reason)
}

func optNilString(v *string) siteapi.OptNilString {
	if v == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*v)
}

func optNilEmailAddress(v *string) siteapi.OptNilEmailAddress {
	if v == nil {
		return siteapi.OptNilEmailAddress{}
	}
	return siteapi.NewOptNilEmailAddress(siteapi.EmailAddress(*v))
}

func optNilEntityID(v *int64) siteapi.OptNilEntityId {
	if v == nil {
		return siteapi.OptNilEntityId{}
	}
	return siteapi.NewOptNilEntityId(siteapi.EntityId(*v))
}

// BroadcastStats nests a Broadcast's delivery figures in the Stats DTO and computes the
// derived engagement rates. Rates are ratios in [0,1]; a zero denominator yields 0.
func BroadcastStats(st broadcasts.Stats) siteapi.SiteBroadcastStats {
	return siteapi.SiteBroadcastStats{
		RecipientsTotal:   narrow.Int32(st.Recipients),
		SentCount:         narrow.Int32(st.Sent),
		OpenedCount:       narrow.Int32(st.Opened),
		ClickedCount:      narrow.Int32(st.Clicked),
		UnsubscribedCount: narrow.Int32(st.Unsubscribed),
		FailedCount:       narrow.Int32(st.Failed),
		DeliveredCount:    narrow.Int32(st.Delivered),
		BouncedCount:      narrow.Int32(st.Bounced),
		ComplainedCount:   narrow.Int32(st.Complained),
		CapturedCount:     narrow.Int32(st.Captured),
		DeliveryRate:      broadcasts.Ratio(st.Sent, st.Recipients),
		OpenRate:          st.OpenRate(),
		ClickRate:         st.ClickRate(),
		ClickToOpenRate:   broadcasts.Ratio(st.Clicked, st.Opened),
		UnsubscribeRate:   broadcasts.Ratio(st.Unsubscribed, st.Sent),
		FailureRate:       broadcasts.Ratio(st.Failed, st.Recipients),
		Revenue:           MessageRevenue(st.Revenue),
	}
}

// MessageRevenue is the Orders and net revenue credited to a Broadcast or an Automation, in
// the Reporting currency (nil until chosen: the figures are then zero). f holds one id: a
// list item narrows it with Only.
func MessageRevenue(f orders.Figures) siteapi.SiteMessageRevenue {
	var c orders.Credited
	for _, v := range f.ByID {
		c = v
	}
	return siteapi.SiteMessageRevenue{
		ReportingCurrency: nilCurrencyCode(f.Currency),
		Orders:            narrow.Int32(c.Orders),
		Amount:            c.Amount,
	}
}

// workspaceTimezone is the Workspace time zone, UTC while an Owner has not set one (ADR 0057).
func workspaceTimezone(v *string) siteapi.TimeZoneName {
	return siteapi.TimeZoneName(reportday.Name(v))
}

// nilCurrencyCode is the Reporting currency, null until an Owner or Admin chooses one (ADR 0064).
func nilCurrencyCode(v *string) siteapi.NilCurrencyCode {
	if v == nil {
		return siteapi.NilCurrencyCode{Null: true}
	}
	return siteapi.NewNilCurrencyCode(siteapi.CurrencyCode(*v))
}

// nilTimeZone is the User time zone (ADR 0068), null while the app follows the browser.
func nilTimeZone(v *string) siteapi.NilTimeZoneName {
	if v == nil {
		return siteapi.NilTimeZoneName{Null: true}
	}
	return siteapi.NewNilTimeZoneName(siteapi.TimeZoneName(*v))
}

func attributionWindowDays(v int) siteapi.AttributionWindowDays {
	return siteapi.AttributionWindowDays(narrow.Int32(v))
}

func optNilTimeZone(v *string) siteapi.OptNilTimeZoneName {
	if v == nil {
		return siteapi.OptNilTimeZoneName{}
	}
	return siteapi.NewOptNilTimeZoneName(siteapi.TimeZoneName(*v))
}

func optNilTimestamp(v *time.Time) siteapi.OptNilTimestamp {
	if v == nil {
		return siteapi.OptNilTimestamp{}
	}
	return siteapi.NewOptNilTimestamp(siteapi.Timestamp(*v))
}

func contactCustomFields(m map[string]any) siteapi.OptNilSiteContactResourceCustomFields {
	if len(m) == 0 {
		return siteapi.OptNilSiteContactResourceCustomFields{}
	}
	fields := make(siteapi.SiteContactResourceCustomFields, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		fields[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilSiteContactResourceCustomFields(fields)
}

// templateDoc is a Template's stored editor document as the opaque object of the contract; a
// Template written as HTML has none.
func templateDoc(doc json.RawMessage) siteapi.OptNilSiteEmailTemplateResourceDoc {
	if len(doc) == 0 {
		return siteapi.OptNilSiteEmailTemplateResourceDoc{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return siteapi.OptNilSiteEmailTemplateResourceDoc{}
	}
	out := make(siteapi.SiteEmailTemplateResourceDoc, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return siteapi.NewOptNilSiteEmailTemplateResourceDoc(out)
}

// broadcastDoc is a Broadcast's stored editor document as the opaque object of the contract; a
// Broadcast written as HTML has none.
func broadcastDoc(doc json.RawMessage) siteapi.OptNilSiteBroadcastResourceDoc {
	if len(doc) == 0 {
		return siteapi.OptNilSiteBroadcastResourceDoc{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return siteapi.OptNilSiteBroadcastResourceDoc{}
	}
	out := make(siteapi.SiteBroadcastResourceDoc, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return siteapi.NewOptNilSiteBroadcastResourceDoc(out)
}

func eventProperties(m map[string]any) siteapi.OptNilSiteEventResourceProperties {
	if len(m) == 0 {
		return siteapi.OptNilSiteEventResourceProperties{}
	}
	props := make(siteapi.SiteEventResourceProperties, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		props[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilSiteEventResourceProperties(props)
}

// automationVersionOrNull is a version of the Automation, or null when it has none.
func automationVersionOrNull(c Converter, v *ent.AutomationVersion) siteapi.NilSiteAutomationVersion {
	if v == nil {
		return siteapi.NilSiteAutomationVersion{Null: true}
	}
	return siteapi.NewNilSiteAutomationVersion(c.AutomationVersionToResource(v))
}

// automationSteps decodes the steps of an Automation version into the typed steps DTO.
// Malformed steps yield no steps rather than an error.
func automationSteps(v *ent.AutomationVersion) []siteapi.SiteAutomationStep {
	stored, err := automations.Decode(v.Steps)
	if err != nil {
		return nil
	}
	steps := make([]siteapi.SiteAutomationStep, 0, len(stored))
	for i := range stored {
		s := &stored[i]
		step := siteapi.SiteAutomationStep{Type: siteapi.SiteAutomationStepType(s.Type)}
		switch s.Type {
		case automations.StepWait:
			step.Seconds = siteapi.NewOptInt32(narrow.Int32(s.Seconds))
		case automations.StepApplyTag, automations.StepRemoveTag:
			step.Tag = siteapi.NewOptString(s.Tag)
		case automations.StepHTTP:
			// Masked: the URL, header values and signing secret never leave the server.
			step.ID = siteapi.NewOptString(s.ID)
			step.Body = siteapi.NewOptString(s.Body)
			step.HasSecret = siteapi.NewOptBool(s.Secret != "")
			step.Headers = lo.Map(s.Headers, func(h automations.Header, _ int) siteapi.SiteAutomationHttpHeader {
				return siteapi.SiteAutomationHttpHeader{Name: h.Name}
			})
		default:
			step.Subject = siteapi.NewOptString(s.Subject)
			step.Body = siteapi.NewOptString(s.Body)
			if doc := convert.DocObject[siteapi.SiteAutomationStepDoc](s.Doc); doc != nil {
				step.Doc = siteapi.NewOptSiteAutomationStepDoc(doc)
				step.EditorVersion = siteapi.NewOptString(s.EditorVersion)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// segmentCondition decodes the stored rule group into the typed DTO. A segment with no
// definition (or one that no longer decodes) reads as having none.
func segmentCondition(def *string) siteapi.OptSiteConditionGroup {
	if def == nil || *def == "" {
		return siteapi.OptSiteConditionGroup{}
	}
	var group siteapi.SiteConditionGroup
	if err := condition.Decode(*def, &group); err != nil {
		return siteapi.OptSiteConditionGroup{}
	}
	return siteapi.NewOptSiteConditionGroup(group)
}

func optTimestamp(v *time.Time) siteapi.OptTimestamp {
	if v == nil {
		return siteapi.OptTimestamp{}
	}
	return siteapi.NewOptTimestamp(siteapi.Timestamp(*v))
}

func optInt32(v *int) siteapi.OptInt32 {
	if v == nil {
		return siteapi.OptInt32{}
	}
	return siteapi.NewOptInt32(narrow.Int32(*v))
}

func optString(v *string) siteapi.OptString {
	if v == nil {
		return siteapi.OptString{}
	}
	return siteapi.NewOptString(*v)
}

func optMembershipSource(v *membership.Source) siteapi.OptSiteMembershipSource {
	if v == nil {
		return siteapi.OptSiteMembershipSource{}
	}
	return siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSource(*v))
}

// optConsentDecision is a User's current answer to an optional consent, absent with none; it is
// never `shown`, which only a notice is.
func optConsentDecision(v *consentrecord.Decision) siteapi.OptSiteOptionalConsentDecision {
	if v == nil {
		return siteapi.OptSiteOptionalConsentDecision{}
	}
	return siteapi.NewOptSiteOptionalConsentDecision(siteapi.SiteOptionalConsentDecision(*v))
}

func optTrackingReason(v *sendingdomain.TrackingReason) siteapi.OptSiteTrackingReason {
	if v == nil {
		return siteapi.OptSiteTrackingReason{}
	}
	return siteapi.NewOptSiteTrackingReason(siteapi.SiteTrackingReason(*v))
}

// tagNames lists the names of a contact's loaded tags, in the order they were read.
func tagNames(tags []*ent.Tag) []string {
	return lo.Map(tags, func(t *ent.Tag, _ int) string { return t.Name })
}

// timestampOf renders a time the schema keeps nullable but the contract always shows.
func timestampOf(t *time.Time) siteapi.Timestamp {
	if t == nil {
		return siteapi.Timestamp{}
	}
	return siteapi.Timestamp(*t)
}

// eraseAt is the date a discarded Workspace's data will be erased.
func eraseAt(discardedAt *time.Time) siteapi.Timestamp {
	if discardedAt == nil {
		return siteapi.Timestamp{}
	}
	return siteapi.Timestamp(workspacedeletion.EraseDate(*discardedAt))
}

// inviterEmail is the inviting User's address when the inviter edge is loaded.
func inviterEmail(inv ent.Invitation) siteapi.OptNilString {
	if inv.Edges.Inviter == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(inv.Edges.Inviter.Email)
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(r ent.AutomationRun) siteapi.OptSiteEnrollmentFailure {
	if r.FailureKind == nil {
		return siteapi.OptSiteEnrollmentFailure{}
	}
	return siteapi.NewOptSiteEnrollmentFailure(siteapi.SiteEnrollmentFailure{
		Kind:    siteapi.SiteEnrollmentFailureKind(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

// joinDomainRecord is the TXT record the Owner publishes to prove the domain.
func joinDomainRecord(d ent.JoinDomain) siteapi.SiteDnsRecord {
	rec := joindomains.RecordOf(&d)
	return siteapi.SiteDnsRecord{Type: siteapi.SiteDnsRecordTypeTXT, Host: rec.Host, Value: rec.Value}
}

func joinDomainVerified(d ent.JoinDomain) bool { return joindomains.Verified(&d) }

func dnsRecordType(sendingdomains.Record) siteapi.SiteDnsRecordType {
	return siteapi.SiteDnsRecordTypeTXT
}

func trackingRecordType(sendingdomains.TrackingRecord) siteapi.SiteTrackingRecordType {
	return siteapi.SiteTrackingRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkit.Kit) siteapi.OptNilTimestamp {
	if !k.Saved {
		return siteapi.OptNilTimestamp{}
	}
	return siteapi.NewOptNilTimestamp(siteapi.Timestamp(k.UpdatedAt))
}

func auditActor(v auditapi.View) siteapi.SiteAuditActor {
	return siteapi.SiteAuditActor{Kind: siteapi.SiteAuditActorKind(v.ActorKind), ID: optNilString(v.ActorID), Name: optNilString(v.ActorName)}
}

func auditTarget(v auditapi.View) siteapi.SiteAuditTarget {
	return siteapi.SiteAuditTarget{Type: v.TargetType, ID: optNilString(v.TargetID), Name: optNilString(v.TargetName)}
}

func auditDiff(d map[string]jx.Raw) siteapi.OptNilSiteAuditEntryResourceDiff {
	if d == nil {
		return siteapi.OptNilSiteAuditEntryResourceDiff{}
	}
	return siteapi.NewOptNilSiteAuditEntryResourceDiff(d)
}

// eventTypes shows an endpoint with no event filter as an empty list, never null.
func eventTypes(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(e ent.WebhookEndpoint) siteapi.OptSiteWebhookFailure {
	if e.ConsecutiveFailures == nil || e.FailingSince == nil || e.LastFailureAt == nil || e.LastErrorKind == nil {
		return siteapi.OptSiteWebhookFailure{}
	}
	return siteapi.NewOptSiteWebhookFailure(siteapi.SiteWebhookFailure{
		ConsecutiveFailures: narrow.Int32(*e.ConsecutiveFailures),
		Since:               siteapi.Timestamp(*e.FailingSince),
		LastFailureAt:       siteapi.Timestamp(*e.LastFailureAt),
		Kind:                siteapi.SiteWebhookErrorKind(*e.LastErrorKind),
		Status:              optInt32(e.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(v *webhookendpoint.DisabledReason) siteapi.OptSiteWebhookDisabledReason {
	if v == nil {
		return siteapi.OptSiteWebhookDisabledReason{}
	}
	return siteapi.NewOptSiteWebhookDisabledReason(siteapi.SiteWebhookDisabledReason(*v))
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) siteapi.OptNilString {
	if s == "" {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(s)
}

func smtpConfigKind(integrations.RedactedSMTP) siteapi.SiteSmtpConfigKind {
	return siteapi.SiteSmtpConfigKindSMTP
}

func sesConfigKind(integrations.RedactedSES) siteapi.SiteSesConfigKind {
	return siteapi.SiteSesConfigKindSes
}

// sendingDomainVerified derives the verified flag from the verified and lost moments.
func sendingDomainVerified(d ent.SendingDomain) bool { return dkimstate.Is(&d) }

// webhookEnabled derives the enabled flag: an endpoint is enabled until it has a disabled moment.
func webhookEnabled(e ent.WebhookEndpoint) bool { return e.DisabledAt == nil }

// ruleTriggerEvents names a Rule version's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(v *ent.RuleVersion) []string { return rules.Triggers(v) }

// ruleVersionOrNull is a version of the Rule, or null when it has none.
func ruleVersionOrNull(c Converter, v *ent.RuleVersion) siteapi.NilSiteRuleVersion {
	if v == nil {
		return siteapi.NilSiteRuleVersion{Null: true}
	}
	return siteapi.NewNilSiteRuleVersion(c.RuleVersionToResource(v))
}

// ruleOptString is a text that is absent when empty.
func ruleOptString(s string) siteapi.OptString {
	if s == "" {
		return siteapi.OptString{}
	}
	return siteapi.NewOptString(s)
}

// ruleActions decodes a Rule's stored Actions into the typed DTO. Stored text the module cannot
// decode (only ever written by the module itself) yields none, and is logged: a converter has
// no way to fail a response, and nobody reading the page can act on it.
func ruleActions(stored string) []siteapi.SiteRuleAction {
	actions, err := rules.DecodeActions(stored)
	if err != nil {
		slog.Error("rule: stored actions cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(actions, func(a rules.Action, _ int) siteapi.SiteRuleAction {
		out := siteapi.SiteRuleAction{Type: siteapi.SiteRuleActionType(a.Type), Tag: ruleOptString(a.Tag)}
		if a.Type == rules.ActionEnroll {
			out.AutomationId = siteapi.NewOptEntityId(siteapi.EntityId(a.AutomationID))
		}
		if a.Type == rules.ActionHTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = ruleOptString(a.ID)
			out.Body = ruleOptString(a.Body)
			out.HasSecret = siteapi.NewOptBool(a.Secret != "")
			out.Headers = lo.Map(a.Headers, func(h httpcall.Header, _ int) siteapi.SiteAutomationHttpHeader {
				return siteapi.SiteAutomationHttpHeader{Name: h.Name}
			})
		}
		return out
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO; stored text the module
// cannot decode yields none and is logged, as ruleActions does.
func ruleResults(stored string) []siteapi.SiteRuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) siteapi.SiteRuleActionResult {
		return siteapi.SiteRuleActionResult{
			Type:   siteapi.SiteRuleActionType(r.Type),
			Status: siteapi.SiteRuleActionStatus(r.Status),
			Detail: ruleOptString(r.Detail),
		}
	})
}

func optNilContactSource(v *contact.Source) siteapi.OptNilContactExportSource {
	if v == nil {
		return siteapi.OptNilContactExportSource{}
	}
	return siteapi.NewOptNilContactExportSource(siteapi.ContactExportSource(*v))
}
