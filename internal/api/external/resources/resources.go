// Package resources holds goverter-generated mappers from ent entities to
// external API resources, plus the custom conversions goverter cannot infer.
// The generated implementation (ConverterImpl) lives in converter_gen.go;
// consumers instantiate it (see internal/api/external).
package resources

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"

	"github.com/go-faster/jx"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

// automationVersionOrNull is a version of the Automation, or null when it has none.
func automationVersionOrNull(c Converter, v *ent.AutomationVersion) (externalapi.NilAutomationVersion, error) {
	if v == nil {
		return externalapi.NilAutomationVersion{Null: true}, nil
	}
	res, err := c.AutomationVersionToResource(v)
	if err != nil {
		return externalapi.NilAutomationVersion{}, err
	}
	return externalapi.NewNilAutomationVersion(res), nil
}

// automationPath maps a version's steps into the typed path DTO, through the automations module's
// masked view of each step.
func automationPath(c Converter, steps automationstep.Path) (externalapi.AutomationPath, error) {
	view, err := automations.View(steps)
	if err != nil {
		return externalapi.AutomationPath{}, err
	}
	return c.AutomationPathToResource(view)
}

// automationStepViews wraps each Step's view of the root path in the arm of its kind (stepSum).
func automationStepViews(c Converter, views []automations.StepView) ([]externalapi.AutomationStep, error) {
	out := make([]externalapi.AutomationStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, externalapi.AutomationStep{OneOf: sum})
	}
	return out, nil
}

// pathStepViews wraps each Step's view of a fork's path in the arm of its kind (stepSum): the
// contract's union of a fork's path has the arms of the root's.
func pathStepViews(c Converter, views []automations.StepView) ([]externalapi.PathStep, error) {
	out := make([]externalapi.PathStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, externalapi.PathStep{OneOf: c.PathStepSum(sum)})
	}
	return out, nil
}

// splitPathStepViews wraps each Step's view of a Split's path in the arm of its kind (stepSum): the
// contract's union of a Split's path has the arms of the root's.
func splitPathStepViews(c Converter, views []automations.StepView) ([]externalapi.SplitPathStep, error) {
	out := make([]externalapi.SplitPathStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, externalapi.SplitPathStep{OneOf: c.SplitPathStepSum(sum)})
	}
	return out, nil
}

// stepSum is the arm of the Step's view: a oneOf is beyond goverter, so only the switch is written
// here. A view with no arm fails the response, never shown as an empty Step.
func stepSum(c Converter, v automations.StepView) (externalapi.AutomationStepSum, error) {
	switch v := v.(type) {
	case automations.EmailView:
		return externalapi.NewEmailStepAutomationStepSum(c.EmailStepToResource(v)), nil
	case automations.TelegramView:
		return externalapi.NewTelegramStepAutomationStepSum(c.TelegramStepToResource(v)), nil
	case automations.SMSView:
		return externalapi.NewSmsStepAutomationStepSum(c.SmsStepToResource(v)), nil
	case automations.WaitView:
		return externalapi.NewWaitStepAutomationStepSum(c.WaitStepToResource(v)), nil
	case automations.WaitUntilView:
		w, err := c.WaitUntilStepToResource(v)
		if err != nil {
			return externalapi.AutomationStepSum{}, err
		}
		return externalapi.NewWaitUntilStepAutomationStepSum(w), nil
	case automations.TagView:
		// The module shows both tag kinds as one view; the union has an arm for each.
		switch v.Type {
		case automationstep.KindApplyTag:
			return externalapi.NewApplyTagStepAutomationStepSum(c.ApplyTagStepToResource(v)), nil
		case automationstep.KindRemoveTag:
			return externalapi.NewRemoveTagStepAutomationStepSum(c.RemoveTagStepToResource(v)), nil
		default:
			return externalapi.AutomationStepSum{}, fmt.Errorf("a tag step view of kind %q has no arm", v.Type)
		}
	case automations.HTTPView:
		return externalapi.NewHttpStepAutomationStepSum(c.HttpStepToResource(v)), nil
	case automations.EndView:
		return externalapi.NewEndStepAutomationStepSum(c.EndStepToResource(v)), nil
	case automations.BranchView:
		b, err := c.BranchStepToResource(v)
		if err != nil {
			return externalapi.AutomationStepSum{}, err
		}
		return externalapi.NewBranchStepAutomationStepSum(b), nil
	case automations.SplitView:
		sp, err := c.SplitStepToResource(v)
		if err != nil {
			return externalapi.AutomationStepSum{}, err
		}
		return externalapi.NewSplitStepAutomationStepSum(sp), nil
	default:
		return externalapi.AutomationStepSum{}, fmt.Errorf("the step view %T has no arm", v)
	}
}

// optAutomationPath is a Branch's unknown path, absent for none.
func optAutomationPath(c Converter, v *automations.PathView) (externalapi.OptAutomationPath, error) {
	if v == nil {
		return externalapi.OptAutomationPath{}, nil
	}
	p, err := c.AutomationPathToResource(*v)
	if err != nil {
		return externalapi.OptAutomationPath{}, err
	}
	return externalapi.NewOptAutomationPath(p), nil
}

// branchCondition maps a Branch path's Condition into the typed DTO.
func branchCondition(g conditiontree.Branch) (externalapi.BranchCondition, error) {
	return convert.Document[externalapi.BranchCondition](g)
}

// The discriminators of the Step arms.
func emailStepType(automationstep.Kind) externalapi.EmailStepType {
	return externalapi.EmailStepTypeEmail
}
func telegramStepType(automationstep.Kind) externalapi.TelegramStepType {
	return externalapi.TelegramStepTypeTelegram
}
func smsStepType(automationstep.Kind) externalapi.SmsStepType   { return externalapi.SmsStepTypeSMS }
func waitStepType(automationstep.Kind) externalapi.WaitStepType { return externalapi.WaitStepTypeWait }
func waitUntilStepType(automationstep.Kind) externalapi.WaitUntilStepType {
	return externalapi.WaitUntilStepTypeWaitUntil
}
func waitUntilTimeOfDayForm(automationstep.TimeOfDay) externalapi.WaitUntilTimeOfDayForm {
	return externalapi.WaitUntilTimeOfDayFormTimeOfDay
}

// waitUntilMoment is the arm of a Wait until's moment: a oneOf is beyond goverter, so only the
// switch is written here. A moment with no arm fails the response.
func waitUntilMoment(c Converter, u automationstep.Until) (externalapi.WaitUntilMoment, error) {
	switch u := u.(type) {
	case automationstep.TimeOfDay:
		return externalapi.WaitUntilMoment{OneOf: externalapi.NewWaitUntilTimeOfDayWaitUntilMomentSum(c.WaitUntilTimeOfDayToResource(u))}, nil
	case automationstep.OnDate:
		return externalapi.WaitUntilMoment{OneOf: externalapi.NewWaitUntilDateWaitUntilMomentSum(externalapi.WaitUntilDate{
			Form: externalapi.WaitUntilDateFormDate, Field: dateField(u.Field), Recurrence: c.DateRecurrenceToResource(u.Recurrence),
			OffsetDays: externalapi.DateOffsetDays(narrow.Int32(u.OffsetDays)), LocalTime: externalapi.LocalTime(u.LocalTime),
		})}, nil
	case nil:
		return externalapi.WaitUntilMoment{}, errors.New("a wait until without a moment")
	}
	return externalapi.WaitUntilMoment{}, fmt.Errorf("the moment %T has no arm", u)
}

func applyTagStepType(automationstep.Kind) externalapi.ApplyTagStepType {
	return externalapi.ApplyTagStepTypeApplyTag
}
func removeTagStepType(automationstep.Kind) externalapi.RemoveTagStepType {
	return externalapi.RemoveTagStepTypeRemoveTag
}
func httpStepType(automationstep.Kind) externalapi.HttpStepType { return externalapi.HttpStepTypeHTTP }
func endStepType(automationstep.Kind) externalapi.EndStepType   { return externalapi.EndStepTypeEnd }
func branchStepType(automationstep.Kind) externalapi.BranchStepType {
	return externalapi.BranchStepTypeBranch
}
func splitStepType(automationstep.Kind) externalapi.SplitStepType {
	return externalapi.SplitStepTypeSplit
}

func optEditorDocument(doc json.RawMessage) externalapi.OptEditorDocument {
	return convert.OptObject[externalapi.OptEditorDocument](convert.DocObject[externalapi.EditorDocument](doc))
}

// Converter maps ent entities to external API resources. goverter generates the
// implementation; the extends below cover the conversions it can't infer.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend revision
// goverter:extend optNilString
// goverter:extend optNilStrings
// goverter:extend optNilStepID
// goverter:extend optNilSendCap
// goverter:extend optNilOptOutVia
// goverter:extend optNilSmsConfirmation
// goverter:extend optNilSmsOptOut
// goverter:extend optNilEngagementTrackingProvenance
// goverter:extend optNilHoldReason
// goverter:extend optNilEmailAddress
// goverter:extend optNilTimeZone
// goverter:extend optNilCountry
// goverter:extend optNilLocale
// goverter:extend optNilImageURL
// goverter:extend optNilDate
// goverter:extend optNilTimestamp
// goverter:extend optNilEntityID
// goverter:extend optEntityID
// goverter:extend optStepID
// goverter:extend jsonObject
// goverter:extend segmentCondition
// goverter:extend ruleCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend editorDocument
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend maxPerDay
// goverter:extend workspaceTimezone
// goverter:extend omitEmpty
// goverter:extend omitEmptyConfigurationSet
// goverter:extend providerLimitSource
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optEditorDocument
// goverter:extend optTrackingReason
// goverter:extend optSkippedReason
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend nilCurrencyCode
// goverter:extend durationSeconds
// goverter:extend eventAction
// goverter:extend broadcastContent
// goverter:extend nilFloat32
// goverter:extend nilFloat64
// goverter:extend emailContentChannel
// goverter:extend automationPath
// goverter:extend automationStepViews
// goverter:extend emailStepType
// goverter:extend telegramStepType
// goverter:extend smsStepType
// goverter:extend waitStepType
// goverter:extend waitUntilStepType
// goverter:extend waitUntilMoment
// goverter:extend waitUntilTimeOfDayForm
// goverter:extend applyTagStepType
// goverter:extend removeTagStepType
// goverter:extend httpStepType
// goverter:extend endStepType
// goverter:extend branchStepType
// goverter:extend splitStepType
// goverter:extend splitPathStepViews
// goverter:extend pathStepViews
// goverter:extend optAutomationPath
// goverter:extend branchCondition
// goverter:extend telegramContentChannel
// goverter:extend smsContentChannel
type Converter interface {
	ContactToResource(source *ent.Contact) externalapi.ContactResource
	// goverter:useEmptyOnNil:slice
	ContactsToResources(source []*ent.Contact) []externalapi.ContactResource
	// Recipients are counted by Schedule (ADR 0149): the handler attaches them to its answer.
	// goverter:ignore Recipients
	// The unsent are counted by Cancel (ADR 0150): the handler attaches them to its answer.
	// goverter:ignore Unsent
	// goverter:map . Content | broadcastContent
	BroadcastToResource(source *ent.Broadcast) externalapi.BroadcastResource
	// The email content of an email Broadcast, the arm of its content that broadcastContent wraps.
	EmailBroadcastContentToResource(source *ent.Broadcast) externalapi.EmailBroadcastContent
	// The Telegram content of a Telegram Broadcast, the arm of its content that broadcastContent wraps.
	TelegramBroadcastContentToResource(source *ent.Broadcast) externalapi.TelegramBroadcastContent
	// The SMS content of an SMS Broadcast, the arm of its content that broadcastContent wraps.
	SmsBroadcastContentToResource(source *ent.Broadcast) externalapi.SmsBroadcastContent
	// goverter:useEmptyOnNil:slice
	BroadcastsToResources(source []*ent.Broadcast) []externalapi.BroadcastResource

	// The figures are the report's Figures, as the Broadcast's Channel reports them: one it does not
	// report is null.
	// goverter:map Figures.Recipients RecipientsTotal
	// goverter:map Figures.Sent SentCount
	// goverter:map Figures.Skipped SkippedCount
	// goverter:map Figures.Pending PendingCount
	// goverter:map Figures.Failed FailedCount
	// goverter:map Figures.Cancelled CancelledCount
	// goverter:map Figures.Opened OpenedCount
	// goverter:map Figures.Clicked ClickedCount
	// goverter:map Figures.Unsubscribed UnsubscribedCount
	// goverter:map Figures.Delivered DeliveredCount
	// goverter:map Figures.Undelivered UndeliveredCount
	// goverter:map Figures.Bounced BouncedCount
	// goverter:map Figures.Complained ComplainedCount
	// goverter:map Figures.Captured CapturedCount
	// goverter:map Figures.Untracked UntrackedCount
	// goverter:map Figures.OpenRate OpenRate
	// goverter:map Figures.ClickRate ClickRate
	BroadcastReportToResource(source broadcasts.Report) externalapi.BroadcastReport
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:autoMap Credited
	// goverter:map Currency ReportingCurrency
	MessageRevenueToResource(source orders.Revenue) externalapi.MessageRevenue
	AutomationReportToResource(source automations.Report) externalapi.AutomationReport
	// goverter:autoMap ResultCounts
	// goverter:map AverageDuration AverageDurationSeconds
	AutomationPeriodResultsToResource(source automations.PeriodResults) externalapi.AutomationPeriodResults
	// goverter:autoMap ResultCounts
	AutomationResultsPointToResource(source automations.ResultsPoint) externalapi.AutomationResultsPoint
	// The period is the one the request named: the handler attaches it.
	AutomationResultsToResource(source automations.Results) externalapi.AutomationResults
	ApiTokenToInfo(source *ent.ApiToken) externalapi.ApiTokenInfo
	// goverter:useEmptyOnNil:slice
	ApiTokensToInfos(source []*ent.ApiToken) []externalapi.ApiTokenInfo
	SegmentToResource(source *ent.Segment) (externalapi.SegmentResource, error)
	// goverter:useEmptyOnNil:slice
	SegmentsToResources(source []*ent.Segment) ([]externalapi.SegmentResource, error)
	EmailTemplateToResource(source *ent.EmailTemplate) externalapi.TemplateResource
	// goverter:useEmptyOnNil:slice
	EmailTemplatesToResources(source []*ent.EmailTemplate) []externalapi.TemplateResource

	// The traits of a batch item's address (ADR 0117); goverter checks each member.
	// goverter:enum:map Disposable ContactBatchWarningDisposableEmail
	// goverter:enum:map Role ContactBatchWarningRoleEmail
	ContactBatchWarning(source addresscheck.Flag) externalapi.ContactBatchWarning
	ContactBatchWarnings(source []addresscheck.Flag) []externalapi.ContactBatchWarning

	// A lint finding (ADR 0040) names its element and detail only when the check has one.
	// goverter:map Element Element | optNonEmpty
	// goverter:map Detail Detail | optNonEmpty
	LintFindingToResource(source emailrender.Finding) externalapi.LintFinding
	// goverter:useEmptyOnNil:slice
	LintFindingsToResource(source []emailrender.Finding) []externalapi.LintFinding

	// The editor's document described to an agent (ADR 0040); the example is an opaque object.
	// goverter:map Example Example | schemaExample
	// goverter:useEmptyOnNil:map
	EmailSchemaToResource(source emaildoc.AgentDescription) externalapi.EmailSchemaResult
	// goverter:map Content Content | optNonEmpty
	// goverter:map Group Group | optNonEmpty
	// goverter:useEmptyOnNil:map
	EmailSchemaTypeToResource(source emaildoc.AgentType) externalapi.EmailSchemaType
	// goverter:useEmptyOnNil:slice
	ConditionTestsToResource(source condition.Tests) externalapi.CustomFieldConditionTests
	// goverter:enum:transform value
	TextOperatorToResource(source conditiontree.TextOperator) externalapi.TextOperator
	// goverter:enum:transform value
	NumberOperatorToResource(source conditiontree.NumberOperator) externalapi.NumberOperator
	// goverter:enum:transform value
	MomentOperatorToResource(source conditiontree.MomentOperator) externalapi.MomentOperator
	// goverter:enum:transform value
	PresenceOperatorToResource(source conditiontree.PresenceOperator) externalapi.PresenceOperator
	// goverter:map Type ConditionTests | customFieldConditionTests
	CustomFieldToResource(source *ent.CustomField) externalapi.CustomFieldResource
	// goverter:useEmptyOnNil:slice
	CustomFieldsToResources(source []*ent.CustomField) []externalapi.CustomFieldResource
	TagToResource(source *ent.Tag) externalapi.TagResource
	StartLinkToResource(source bindings.StartLink) externalapi.TelegramStartLinkResource
	// goverter:useEmptyOnNil:slice
	TagsToResources(source []*ent.Tag) []externalapi.TagResource
	// The live version and the draft are the edges the automations module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	AutomationToResource(source *ent.Automation) (externalapi.AutomationResource, error)
	// goverter:useEmptyOnNil:slice
	AutomationsToResources(source []*ent.Automation) ([]externalapi.AutomationResource, error)
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map Steps | automationPath
	// goverter:map . Trigger | automationTrigger
	// goverter:map . ExitEvents | automationExits
	// goverter:map Goal GoalEvent | automationGoal
	AutomationVersionToResource(source *ent.AutomationVersion) (externalapi.AutomationVersion, error)
	// goverter:map Steps | automationStepViews
	AutomationPathToResource(source automations.PathView) (externalapi.AutomationPath, error)
	// The arms of a Step, the module's masked view of each kind, which automationStepViews wraps.
	// goverter:map IntegrationID IntegrationId
	EmailStepToResource(source automations.EmailView) externalapi.EmailStep
	// goverter:map IntegrationID IntegrationId
	TelegramStepToResource(source automations.TelegramView) externalapi.TelegramStep
	// goverter:map IntegrationID IntegrationId
	SmsStepToResource(source automations.SMSView) externalapi.SmsStep
	WaitStepToResource(source automations.WaitView) externalapi.WaitStep
	WaitUntilStepToResource(source automations.WaitUntilView) (externalapi.WaitUntilStep, error)
	// goverter:map . Form | waitUntilTimeOfDayForm
	// goverter:useEmptyOnNil:slice
	WaitUntilTimeOfDayToResource(source automationstep.TimeOfDay) externalapi.WaitUntilTimeOfDay
	// goverter:enum:transform value
	WeekdayToResource(source localcalendar.Weekday) externalapi.Weekday
	ApplyTagStepToResource(source automations.TagView) externalapi.ApplyTagStep
	RemoveTagStepToResource(source automations.TagView) externalapi.RemoveTagStep
	HttpStepToResource(source automations.HTTPView) externalapi.HttpStep
	EndStepToResource(source automations.EndView) externalapi.EndStep
	BranchStepToResource(source automations.BranchView) (externalapi.BranchStep, error)
	BranchPathToResource(source automations.BranchPathView) (externalapi.BranchPath, error)
	// A Step of the root's union as a Step of a fork's path: the two unions have the same arms, so
	// an arm one of them lacks fails generation (pathStepViews).
	// goverter:skipCopySameType
	PathStepSum(source externalapi.AutomationStepSum) externalapi.PathStepSum
	// goverter:enum:transform value
	PathStepType(source externalapi.AutomationStepSumType) externalapi.PathStepSumType
	// A Step of the root's union as a Step of a Split's path, as PathStepSum (splitPathStepViews).
	// goverter:skipCopySameType
	SplitPathStepSum(source externalapi.AutomationStepSum) externalapi.SplitPathStepSum
	// goverter:enum:transform value
	SplitPathStepType(source externalapi.AutomationStepSumType) externalapi.SplitPathStepSumType
	SplitStepToResource(source automations.SplitView) (externalapi.SplitStep, error)
	SplitPathToResource(source automations.SplitPathView) (externalapi.SplitPath, error)
	// goverter:ignore Value
	AutomationHttpHeaderToResource(source automations.HeaderName) externalapi.AutomationHttpHeader
	TelegramButtonToResource(source telegramrender.Button) externalapi.TelegramButton

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Provider limit from providerlimit.Module.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore ProviderLimit
	IntegrationToResource(source *ent.Integration) externalapi.IntegrationResource

	// A Captured message names its From as the contract does, and the nullable parts as null.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToResource(source *ent.CapturedMessage) externalapi.CapturedMessageResource
	// The list leaves the content out.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToSummary(source *ent.CapturedMessage) externalapi.CapturedMessageSummary
	// goverter:useEmptyOnNil:slice
	CapturedMessagesToSummaries(source []*ent.CapturedMessage) []externalapi.CapturedMessageSummary
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) externalapi.SmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) externalapi.SesConfig
	// goverter:map . Kind | telegramConfigKind
	// goverter:map BotID BotId
	TelegramConfigToResource(source integrations.RedactedTelegram) externalapi.TelegramConfig
	// goverter:enum:transform value
	EntryPolicyToResource(source automationversion.EntryPolicy) externalapi.EntryPolicy
	// goverter:enum:transform value
	DateRecurrenceToResource(source localcalendar.Recurrence) externalapi.DateRecurrence
	// goverter:enum:transform value
	SmsFromKindToResource(source messaging.SMSFromKind) externalapi.SmsFromKind
	// goverter:map . Kind | twilioConfigKind
	// goverter:map AccountSID AccountSid
	// goverter:map . ApiKeySid | twilioAPIKeySID
	TwilioConfigToResource(source integrations.RedactedTwilio) externalapi.TwilioConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) externalapi.ProviderLimitStatus
	// goverter:map Sent24h SentLast24h
	SendCapToResource(source providerlimit.SendCapStatus) externalapi.SendCap
	// The Send cap that applies (outboundmessages.Module.SendCapStatus), null when none does.
	SendCapStatusToResource(source *providerlimit.SendCapStatus) externalapi.OptNilSendCap

	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	// The Send cap shown is the one the send path enforces (outboundmessages.Module.SendCapStatus), which
	// the mapper cannot see: the handler attaches it through SendCapStatusToResource.
	// goverter:ignore SendCap
	WorkspaceToResource(source *ent.Workspace) externalapi.WorkspaceResource

	// The live version and the draft are the edges the rules module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) (externalapi.RuleResource, error)
	// goverter:useEmptyOnNil:slice
	RulesToResources(source []*ent.Rule) ([]externalapi.RuleResource, error)

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) (externalapi.RuleVersion, error)

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) externalapi.RuleFiringResource
	// goverter:useEmptyOnNil:slice
	RuleFiringsToResources(source []*ent.RuleFiring) []externalapi.RuleFiringResource

	RuleDryRunToResource(source rules.DryRun) externalapi.RuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	// goverter:map Occurrence | enrollmentOccurrence
	EnrollmentToResource(source *ent.AutomationRun) externalapi.EnrollmentResource
	// The store names the triggering Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	SkippedEntryToResource(source *ent.AutomationSkippedEntry) externalapi.SkippedEntryResource
	// goverter:useEmptyOnNil:slice
	EnrollmentsToResources(source []*ent.AutomationRun) []externalapi.EnrollmentResource

	// The signing secret is deliberately never part of an /api response; the failure state
	// is derived from the failure columns and a nil event filter renders as an empty list.
	// goverter:useEmptyOnNil:slice
	// goverter:map . Failure | webhookFailure
	// goverter:map . Enabled | webhookEnabled
	WebhookToResource(source *ent.WebhookEndpoint) externalapi.WebhookResource

	// The DNS records are derived by the sendingdomains module, which the mapper cannot
	// see: the handler attaches them. The private key is never exposed.
	// goverter:ignore DkimRecord
	// goverter:ignore SpfRecord
	// goverter:ignore DmarcRecord
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) externalapi.SendingDomainResource
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) externalapi.DnsRecord

	// The CNAME target is the module's, which the mapper cannot see: the handler attaches it.
	// goverter:ignore Record OwnershipRecord
	// goverter:map CheckedAt LastCheckedAt
	TrackingDomainToResource(source *ent.TrackingDomain) externalapi.TrackingDomainResource
	// goverter:map . Type | cnameRecordType
	CnameRecordToResource(source trackingdomains.Record) externalapi.CnameRecord
	// goverter:map . Type | ownershipRecordType
	OwnershipRecordToResource(source trackingdomains.OwnershipRecord) externalapi.DnsRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkits.Kit) externalapi.BrandKitResource

	// goverter:map . Actor
	// goverter:map . Target
	AuditEntryToResource(source auditlog.View) externalapi.AuditEntryResource
	// goverter:useEmptyOnNil:slice
	AuditEntriesToResources(source []auditlog.View) []externalapi.AuditEntryResource
	// An entry's actor and target are columns of the entry, nested in the contract.
	// goverter:map ActorKind Kind
	// goverter:map ActorID ID
	// goverter:map ActorName Name
	AuditActorToResource(source auditlog.View) externalapi.AuditActor
	// goverter:map TargetType Type
	// goverter:map TargetID ID
	// goverter:map TargetName Name
	AuditTargetToResource(source auditlog.View) externalapi.AuditTarget

	SuppressionToResource(source *ent.Suppression) externalapi.SuppressionResource
	UnsubscribeToResource(source *ent.Unsubscribe) externalapi.UnsubscribeResource
	ConfirmationToResource(source *ent.Confirmation) externalapi.ConfirmationResource
	SmsConsentToResource(source confirmations.ContactSMS) externalapi.SmsConsent
	SmsOptOutToResource(source *confirmations.SMSOptOut) externalapi.SmsOptOut
	// goverter:map CreatedAt ConfirmedAt
	SmsConfirmationToResource(source *ent.Confirmation) externalapi.SmsConfirmation

	// The Engagement tracking consent of a Destination (ADR 0120), and what an observation did.
	EngagementTrackingConsentToResource(source engagementtracking.Consent) externalapi.EngagementTrackingConsent
	ContactEngagementTrackingToResource(source engagementtracking.ContactConsents) externalapi.ContactEngagementTracking
	EngagementTrackingRecordedToResource(source engagementtracking.Recorded) externalapi.EngagementTrackingObservationResult
	// goverter:enum:transform value
	EngagementTrackingChannelToResource(source channels.Channel) externalapi.EngagementTrackingChannel
	// goverter:enum:transform value
	EngagementTrackingReadingToResource(source engagementtracking.Reading) externalapi.EngagementTrackingReading
	// goverter:enum:transform value
	EngagementTrackingProvenanceToResource(source engagementtrackingconsent.Provenance) externalapi.EngagementTrackingProvenance
	// goverter:enum:transform value
	EngagementTrackingDecisionToResource(source engagementtracking.Decision) externalapi.EngagementTrackingDecision
	// goverter:enum:transform value
	EngagementTrackingOutcomeToResource(source engagementtracking.Outcome) externalapi.EngagementTrackingOutcome

	// The Data export members (ADR 0021): the contract's typed projection of the
	// stored rows, so the ent JSON tags are never the public format.
	ContactToExport(source *ent.Contact) externalapi.ContactExportContact
	TagToExport(source *ent.Tag) externalapi.ContactExportTag
	VisitorToExport(source *ent.Visitor) externalapi.ContactExportVisitor
	// goverter:map . Properties | eventProperties
	EventToExport(source *ent.Event) externalapi.ContactExportEvent
	UnsubscribeToExport(source *ent.Unsubscribe) externalapi.ContactExportUnsubscribe
	SuppressionToExport(source *ent.Suppression) externalapi.ContactExportSuppression
	ConfirmationToExport(source *ent.Confirmation) externalapi.ContactExportConfirmation
	OutboundMessageToExport(source *ent.OutboundMessage) externalapi.ContactExportOutboundMessage
	BroadcastRecipientToExport(source *ent.BroadcastRecipient) externalapi.ContactExportBroadcastRecipient

	// Pages: a list response is the module's pagination.Page mapped whole, its items through
	// the list methods above. Items the handler maps itself (an extra per item) arrive as a
	// page of resources (Page.Map), an unpaginated list as pagination.Whole.
	// goverter:useEmptyOnNil:slice
	ContactsListPage(source pagination.Page[*ent.Contact]) *externalapi.ContactsListOK
	// goverter:useEmptyOnNil:slice
	SegmentsListPage(source pagination.Page[*ent.Segment]) (*externalapi.SegmentsListOK, error)
	// goverter:useEmptyOnNil:slice
	TemplatesListPage(source pagination.Page[*ent.EmailTemplate]) *externalapi.TemplatesListOK
	// goverter:useEmptyOnNil:slice
	BroadcastsListPage(source pagination.Page[*ent.Broadcast]) *externalapi.BroadcastsListOK
	// goverter:useEmptyOnNil:slice
	AutomationsListPage(source pagination.Page[*ent.Automation]) (*externalapi.AutomationsListOK, error)
	// goverter:useEmptyOnNil:slice
	SkippedEntriesListPage(source pagination.Page[*ent.AutomationSkippedEntry]) *externalapi.SkippedEntriesListOK
	// goverter:useEmptyOnNil:slice
	EnrollmentsListPage(source pagination.Page[*ent.AutomationRun]) *externalapi.EnrollmentsListOK
	// goverter:useEmptyOnNil:slice
	RulesListPage(source pagination.Page[*ent.Rule]) (*externalapi.RulesListOK, error)
	// goverter:useEmptyOnNil:slice
	RuleFiringsListPage(source pagination.Page[*ent.RuleFiring]) *externalapi.RuleFiringsListOK
	// goverter:useEmptyOnNil:slice
	CapturedMessagesListPage(source pagination.Page[*ent.CapturedMessage]) *externalapi.CapturedMessagesListOK
	// goverter:useEmptyOnNil:slice
	TagsListPage(source pagination.Page[*ent.Tag]) *externalapi.TagsListOK
	// goverter:useEmptyOnNil:slice
	TagsListForContactPage(source pagination.Page[*ent.Tag]) *externalapi.TagsListForContactOK
	// goverter:useEmptyOnNil:slice
	CustomFieldsListPage(source pagination.Page[*ent.CustomField]) *externalapi.CustomFieldsListOK
	// goverter:useEmptyOnNil:slice
	EventActionsListPage(source pagination.Page[string]) *externalapi.EventActionsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	WebhooksListPage(source pagination.Page[externalapi.WebhookResource]) *externalapi.WebhooksListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	SendingDomainsListPage(source pagination.Page[externalapi.SendingDomainResource]) *externalapi.SendingDomainsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	IntegrationsListPage(source pagination.Page[externalapi.IntegrationResource]) *externalapi.IntegrationsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	PresetsListPage(source pagination.Page[externalapi.PresetResource]) *externalapi.PresetsListOK
	// The window is the one the request named, which the rates do not carry.
	// goverter:context windowDays
	// goverter:useEmptyOnNil:slice
	SendingDomainRatesListPage(source pagination.Page[reputation.DomainRates], windowDays int32) *externalapi.SendingDomainRatesListOK
	// goverter:context windowDays
	// goverter:map Domain.ID SendingDomainId
	// goverter:map Domain.Domain Domain
	// goverter:map Complaint ComplaintRate
	// goverter:map Bounce BounceRate
	// goverter:map WindowDays | windowDaysOf
	SendingDomainRatesToResource(source reputation.DomainRates, windowDays int32) externalapi.SendingDomainRatesResource
	RateToTriple(source reputation.Rate) externalapi.RateTriple

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map ReportPeriod7Days AutomationReportPeriod7d
	// goverter:enum:map ReportPeriod14Days AutomationReportPeriod14d
	// goverter:enum:map ReportPeriod30Days AutomationReportPeriod30d
	// goverter:enum:map ReportPeriod90Days AutomationReportPeriod90d
	AutomationReportPeriodToResource(source automations.ReportPeriod) externalapi.AutomationReportPeriod
	// goverter:enum:transform value
	AutomationStatusToResource(source automation.Status) externalapi.AutomationStatus
	// goverter:enum:transform value
	BroadcastStatusToResource(source broadcast.Status) externalapi.BroadcastStatus
	// goverter:enum:transform value
	BroadcastChannelToResource(source channels.Channel) externalapi.BroadcastChannel
	// goverter:enum:transform value
	CapturedMessageSurfaceToResource(source capturedmessage.Surface) externalapi.CapturedMessageSurface
	// goverter:enum:transform value
	ContactExportSourceToResource(source contact.Source) externalapi.ContactExportSource
	// goverter:enum:transform value
	OptOutViaToResource(source unsubscribe.Via) externalapi.OptOutVia
	// goverter:enum:transform value
	HoldReasonToResource(source holdreason.Reason) externalapi.HoldReason
	// goverter:enum:transform value
	CustomFieldTypeToResource(source customfield.Type) externalapi.CustomFieldType
	// goverter:enum:transform value
	EnrollmentFailureKindToResource(source automationrun.FailureKind) externalapi.EnrollmentFailureKind
	// goverter:enum:transform value
	EnrollmentStatusToResource(source automationrun.Status) externalapi.EnrollmentStatus
	// goverter:enum:transform value
	SkippedEntryReasonToResource(source automationskippedentry.Reason) externalapi.SkippedEntryReason
	// goverter:enum:transform value
	IntegrationChannelToResource(source channels.Channel) externalapi.IntegrationChannel
	// goverter:enum:transform value
	IntegrationProviderToResource(source providers.Provider) externalapi.IntegrationProvider
	// goverter:enum:transform value
	LintCodeToResource(source emailrender.Code) externalapi.LintCode
	// goverter:enum:transform value
	ProviderLimitSourceToResource(source providerlimit.Source) externalapi.ProviderLimitSource
	// goverter:enum:transform value
	ProviderLimitWarningToResource(source providerlimit.Warning) externalapi.ProviderLimitWarning
	// goverter:enum:transform value
	RuleActionStatusToResource(source rules.Outcome) externalapi.RuleActionStatus
	// goverter:enum:transform value
	RuleActionTypeToResource(source ruleaction.Type) externalapi.RuleActionType
	// goverter:enum:transform value
	RuleDryRunOutcomeToResource(source rules.DryRunOutcome) externalapi.RuleDryRunOutcome
	// goverter:enum:transform value
	RuleStatusToResource(source rule.Status) externalapi.RuleStatus
	// goverter:enum:transform value
	SuppressionChannelToResource(source channels.Channel) externalapi.SuppressionChannel
	// The channel of an Unsubscribe and of a Confirmation.
	// goverter:enum:transform value
	ConsentChannelToResource(source channels.Channel) externalapi.ConsentChannel
	// goverter:enum:transform value
	ConfirmationProvenanceToResource(source confirmation.Provenance) externalapi.ConfirmationProvenance
	// goverter:enum:transform value
	SuppressionReasonToResource(source suppression.Reason) externalapi.SuppressionReason
	// goverter:enum:transform value
	TrackingReasonToResource(source trackingdomain.Reason) externalapi.TrackingReason
	// goverter:enum:transform value
	SkippedReasonToResource(source skipreason.Reason) externalapi.SkippedReason
	// goverter:enum:transform value
	TransactionalSendStatusToResource(source outboundmessages.TransactionalStatus) externalapi.TransactionalSendStatus
	// goverter:enum:transform value
	TrackingStateToResource(source trackingdomain.State) externalapi.TrackingState
	// goverter:enum:transform value
	TrackingDomainRoleToResource(source trackingdomain.Role) externalapi.TrackingDomainRole
	// goverter:enum:transform value
	WebhookDisabledReasonToResource(source webhookendpoint.DisabledReason) externalapi.WebhookDisabledReason
	// What a delivery failed with: the kind a test send answers and a Webhook endpoint keeps.
	// goverter:enum:transform value
	WebhookErrorKindToResource(source webhook.ErrorKind) externalapi.WebhookErrorKind
}

// ExportMapper adapts the Converter to dataexport.Mapper.
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

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

// eventAction is one distinct action name of the Workspace's Events.
func eventAction(action string) externalapi.EventActionResource {
	return externalapi.EventActionResource{Action: action}
}

// maxPerDay narrows a rolling 24-hour limit to the contract's bounded int32.
func maxPerDay(n int) externalapi.MaxPerDay { return externalapi.MaxPerDay(narrow.Int32(n)) }

// workspaceTimezone is the Workspace time zone, UTC while an Owner has not set one (ADR 0057).
func workspaceTimezone(v *string) externalapi.TimeZoneName {
	return externalapi.TimeZoneName(reportday.Name(v))
}

// nilCurrencyCode is the Reporting currency, null until an Owner or Admin chooses one (ADR 0064).
func nilCurrencyCode(v *string) externalapi.NilCurrencyCode {
	return convert.Nil[externalapi.NilCurrencyCode]((*externalapi.CurrencyCode)(v))
}

// durationSeconds is a duration in whole seconds, null when there was nothing to measure.
func durationSeconds(d *time.Duration) externalapi.NilInt64 {
	return convert.Nil[externalapi.NilInt64](convert.WholeSeconds(d))
}

// nilString renders an optional value: null when there is none.
func nilString(v *string) externalapi.NilString {
	return convert.Nil[externalapi.NilString](v)
}

// nilEntityID renders an optional reference: null when there is none.
func nilEntityID(v *int64) externalapi.NilEntityId {
	return convert.Nil[externalapi.NilEntityId]((*externalapi.EntityId)(v))
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) externalapi.NilInt32 {
	return convert.Nil[externalapi.NilInt32](convert.Int32Ptr(v))
}

// providerLimitSource is null exactly when the window has no limit (its source is empty).
func providerLimitSource(c Converter, v providerlimit.Source) externalapi.NilProviderLimitSource {
	if v == "" {
		return externalapi.NilProviderLimitSource{Null: true}
	}
	return externalapi.NewNilProviderLimitSource(c.ProviderLimitSourceToResource(v))
}

// jsonObject renders a stored JSON object (custom fields, event properties): absent when it has
// no keys.
// nilFloat32 renders a figure a Channel may not report: null when it does not.
func nilFloat32(v *float32) externalapi.NilFloat32 {
	return convert.Nil[externalapi.NilFloat32](v)
}

// nilFloat64 is a ratio that is undefined (null) when its denominator is zero.
func nilFloat64(v *float64) externalapi.NilFloat64 {
	return convert.Nil[externalapi.NilFloat64](v)
}

// windowDaysOf is the trailing window, in days, the request named for the rates.
//
// goverter:context windowDays
func windowDaysOf(windowDays int32) int32 { return windowDays }

// eventProperties is an Event's properties as a client sees them, a System event's keys by their
// public form (systemevent.PublicProperties).
func eventProperties(e *ent.Event) externalapi.OptNilJsonObject {
	return jsonObject(systemevent.PublicProperties(e.Action, e.Properties))
}

func jsonObject(m map[string]any) externalapi.OptNilJsonObject {
	return convert.OptObject[externalapi.OptNilJsonObject](convert.RawObject[externalapi.JsonObject](m))
}

// broadcastContent is a Broadcast's content in the arm of its Channel: a oneOf is beyond
// goverter, so only the switch is written here.
func broadcastContent(c Converter, b *ent.Broadcast) externalapi.BroadcastContent {
	switch b.Channel {
	case channels.Email:
		return externalapi.BroadcastContent{OneOf: externalapi.NewEmailBroadcastContentBroadcastContentSum(c.EmailBroadcastContentToResource(b))}
	case channels.Telegram:
		return externalapi.BroadcastContent{OneOf: externalapi.NewTelegramBroadcastContentBroadcastContentSum(c.TelegramBroadcastContentToResource(b))}
	case channels.SMS:
		return externalapi.BroadcastContent{OneOf: externalapi.NewSmsBroadcastContentBroadcastContentSum(c.SmsBroadcastContentToResource(b))}
	default:
		return externalapi.BroadcastContent{}
	}
}

// smsContentChannel is the discriminator of SMS's content arm.
func smsContentChannel(channels.Channel) externalapi.SmsBroadcastContentChannel {
	return externalapi.SmsBroadcastContentChannelSMS
}

// telegramContentChannel is the discriminator of Telegram's content arm.
func telegramContentChannel(channels.Channel) externalapi.TelegramBroadcastContentChannel {
	return externalapi.TelegramBroadcastContentChannelTelegram
}

// emailContentChannel is the discriminator of email's content arm.
func emailContentChannel(channels.Channel) externalapi.EmailBroadcastContentChannel {
	return externalapi.EmailBroadcastContentChannelEmail
}

// editorDocument is a stored editor document (a Template's, a Broadcast's) as the opaque object of
// the contract; a message written as HTML has none.
func editorDocument(doc json.RawMessage) externalapi.OptNilEditorDocument {
	return convert.OptObject[externalapi.OptNilEditorDocument](convert.DocObject[externalapi.EditorDocument](doc))
}

func timestamp(t time.Time) externalapi.Timestamp {
	return externalapi.Timestamp(t)
}

func optNilString(v *string) externalapi.OptNilString {
	return convert.Opt[externalapi.OptNilString](v)
}

// optNilStrings is a list stored as NULL when it has nothing to say (nil), absent then.
func optNilStrings(v []string) externalapi.OptNilStringArray {
	if v == nil {
		return externalapi.OptNilStringArray{}
	}
	return externalapi.NewOptNilStringArray(v)
}

func optNilEmailAddress(v *string) externalapi.OptNilEmailAddress {
	return convert.Opt[externalapi.OptNilEmailAddress]((*externalapi.EmailAddress)(v))
}

// optStepID is a Step id absent when there is none.
func optStepID(v *automationstep.ID) externalapi.OptStepId {
	return convert.Opt[externalapi.OptStepId]((*externalapi.StepId)(v))
}

// optNilStepID is the Step id an Automation message names, null for another kind of message.
func optNilStepID(v *automationstep.ID) externalapi.OptNilString {
	return convert.Opt[externalapi.OptNilString]((*string)(v))
}

// optEntityID is an id absent when there is none.
func optEntityID(v *int64) externalapi.OptEntityId {
	return convert.Opt[externalapi.OptEntityId]((*externalapi.EntityId)(v))
}

func optNilEntityID(v *int64) externalapi.OptNilEntityId {
	return convert.Opt[externalapi.OptNilEntityId]((*externalapi.EntityId)(v))
}

func optNilTimeZone(v *string) externalapi.OptNilTimeZoneName {
	return convert.Opt[externalapi.OptNilTimeZoneName]((*externalapi.TimeZoneName)(v))
}

func optNilCountry(v *string) externalapi.OptNilCountryCode {
	return convert.Opt[externalapi.OptNilCountryCode]((*externalapi.CountryCode)(v))
}

func optNilLocale(v *string) externalapi.OptNilLocaleTag {
	return convert.Opt[externalapi.OptNilLocaleTag]((*externalapi.LocaleTag)(v))
}

func optNilImageURL(v *string) externalapi.OptNilImageUrl {
	return convert.Opt[externalapi.OptNilImageUrl]((*externalapi.ImageUrl)(v))
}

func optNilDate(v *time.Time) externalapi.OptNilDate { return convert.Opt[externalapi.OptNilDate](v) }

func optNilTimestamp(v *time.Time) externalapi.OptNilTimestamp {
	return convert.Opt[externalapi.OptNilTimestamp]((*externalapi.Timestamp)(v))
}

// segmentCondition maps a Segment's Condition into the typed DTO. A stored Condition is checked when it
// is written, so one that does not decode is a broken row, and it fails the response.
func segmentCondition(g conditiontree.Segment) (externalapi.SegmentCondition, error) {
	return convert.Document[externalapi.SegmentCondition](g)
}

// automationTrigger maps a version's Trigger into the typed DTO by its kind: an Event matcher whose
// filter is checked as a stored Condition is (convert.Document).
func automationTrigger(c Converter, v *ent.AutomationVersion) (externalapi.AutomationTrigger, error) {
	switch t := automations.TriggerOf(v).(type) {
	case automations.DateTrigger:
		return externalapi.AutomationTrigger{OneOf: externalapi.NewDateTriggerAutomationTriggerSum(externalapi.DateTrigger{
			Kind: externalapi.DateTriggerKindDate, Field: dateField(t.Date.Field), Recurrence: c.DateRecurrenceToResource(t.Date.Recurrence),
			OffsetDays: externalapi.DateOffsetDays(narrow.Int32(t.Date.OffsetDays)), LocalTime: externalapi.LocalTime(t.Date.LocalTime),
		})}, nil
	case automations.EventTrigger:
		ev := externalapi.EventTrigger{
			Kind: externalapi.EventTriggerKindEvent, Action: t.Matcher.Action,
			EntryPolicy: c.EntryPolicyToResource(v.EntryPolicy), Cooldown: externalapi.EntryCooldown(narrow.Int32(v.CooldownSeconds)),
			EnrollmentKey: optString(v.EnrollmentKey),
		}
		if t.Matcher.Filter != nil {
			f, err := convert.Document[externalapi.EventFilter](*t.Matcher.Filter)
			if err != nil {
				return externalapi.AutomationTrigger{}, err
			}
			ev.Filter = externalapi.NewOptEventFilter(f)
		}
		return externalapi.AutomationTrigger{OneOf: externalapi.NewEventTriggerAutomationTriggerSum(ev)}, nil
	}
	return externalapi.AutomationTrigger{}, fmt.Errorf("version %d has a trigger of no kind", v.ID)
}

// dateField maps a date Trigger's field into the typed DTO by its kind. A stored field is checked
// when it is written, so a kind it does not know is the birthday's never.
func dateField(f datetrigger.Field) externalapi.DateField {
	switch f.Kind {
	case datetrigger.CustomField:
		return externalapi.DateField{OneOf: externalapi.NewCustomFieldDateFieldDateFieldSum(externalapi.CustomFieldDateField{
			Kind: externalapi.CustomFieldDateFieldKindCustomField, Key: f.Key,
		})}
	case datetrigger.Birthday:
	}
	return externalapi.DateField{OneOf: externalapi.NewBirthdayDateFieldDateFieldSum(externalapi.BirthdayDateField{Kind: externalapi.BirthdayDateFieldKindBirthday})}
}

// eventMatcher maps an exit or goal event into the typed DTO, its filter checked as a stored
// Condition is (convert.Document).
func eventMatcher(m eventmatcher.Matcher) (externalapi.EventMatcher, error) {
	out := externalapi.EventMatcher{Action: m.Action}
	if m.Filter != nil {
		f, err := convert.Document[externalapi.EventFilter](*m.Filter)
		if err != nil {
			return externalapi.EventMatcher{}, err
		}
		out.Filter = externalapi.NewOptEventFilter(f)
	}
	return out, nil
}

// automationExits maps a version's exit events into the typed DTO; never nil.
func automationExits(v *ent.AutomationVersion) ([]externalapi.EventMatcher, error) {
	exits := automations.ExitsOf(v)
	out := make([]externalapi.EventMatcher, len(exits))
	for i, e := range exits {
		m, err := eventMatcher(e)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// automationGoal maps a version's goal event into the typed DTO: null when there is none.
func automationGoal(goal *eventmatcher.Matcher) (externalapi.NilEventMatcher, error) {
	if goal == nil {
		return externalapi.NilEventMatcher{Null: true}, nil
	}
	m, err := eventMatcher(*goal)
	if err != nil {
		return externalapi.NilEventMatcher{}, err
	}
	return externalapi.NewNilEventMatcher(m), nil
}

// ruleCondition maps a Rule's Condition into the typed DTO, failing as segmentCondition does.
func ruleCondition(g conditiontree.Rule) (externalapi.RuleCondition, error) {
	return convert.Document[externalapi.RuleCondition](g)
}

func optTimestamp(v *time.Time) externalapi.OptTimestamp {
	return convert.Opt[externalapi.OptTimestamp]((*externalapi.Timestamp)(v))
}

func optInt32(v *int) externalapi.OptInt32 {
	return convert.Opt[externalapi.OptInt32](convert.Int32Ptr(v))
}

func optString(v *string) externalapi.OptString {
	return convert.Opt[externalapi.OptString](v)
}

func optTrackingReason(c Converter, v *trackingdomain.Reason) externalapi.OptTrackingReason {
	return convert.OptVia[externalapi.OptTrackingReason](v, c.TrackingReasonToResource)
}

// optSkippedReason is absent on every message but a skipped one.
func optSkippedReason(c Converter, v *skipreason.Reason) externalapi.OptNilSkippedReason {
	return convert.OptVia[externalapi.OptNilSkippedReason](v, c.SkippedReasonToResource)
}

// enrollmentOccurrence is the occurrence of a date Trigger the enrollment was made for (ADR 0126):
// its anniversary year, or a once date's one occurrence; absent for one an Event or a rule began.
func enrollmentOccurrence(stored *datetrigger.StoredOccurrence) externalapi.OptEnrollmentOccurrence {
	if stored == nil {
		return externalapi.OptEnrollmentOccurrence{}
	}
	var sum externalapi.EnrollmentOccurrenceSum
	switch o := stored.Occurrence().(type) {
	case datetrigger.Yearly:
		sum = externalapi.NewYearlyEnrollmentOccurrenceEnrollmentOccurrenceSum(externalapi.YearlyEnrollmentOccurrence{
			Recurrence: externalapi.YearlyEnrollmentOccurrenceRecurrenceYearly, Year: count(o.Year),
		})
	case datetrigger.Once:
		sum = externalapi.NewOnceEnrollmentOccurrenceEnrollmentOccurrenceSum(externalapi.OnceEnrollmentOccurrence{
			Recurrence: externalapi.OnceEnrollmentOccurrenceRecurrenceOnce,
		})
	}
	return externalapi.NewOptEnrollmentOccurrence(externalapi.EnrollmentOccurrence{OneOf: sum})
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(c Converter, r ent.AutomationRun) externalapi.OptEnrollmentFailure {
	if r.FailureKind == nil {
		return externalapi.OptEnrollmentFailure{}
	}
	return externalapi.NewOptEnrollmentFailure(externalapi.EnrollmentFailure{
		Kind:    c.EnrollmentFailureKindToResource(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

func dnsRecordType(sendingdomains.Record) externalapi.DnsRecordType {
	return externalapi.DnsRecordTypeTXT
}

func cnameRecordType(trackingdomains.Record) externalapi.CnameRecordType {
	return externalapi.CnameRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkits.Kit) externalapi.OptNilTimestamp {
	if !k.Saved {
		return externalapi.OptNilTimestamp{}
	}
	return externalapi.NewOptNilTimestamp(externalapi.Timestamp(k.UpdatedAt))
}

func auditDiff(d map[string]jx.Raw) externalapi.OptNilJsonObject {
	return convert.OptObject[externalapi.OptNilJsonObject](externalapi.JsonObject(d))
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(c Converter, e ent.WebhookEndpoint) externalapi.OptWebhookFailure {
	h := webhookendpoints.HealthOf(&e)
	if h.ConsecutiveFailures == nil || h.FailingSince == nil || h.LastFailureAt == nil || h.LastErrorKind == nil {
		return externalapi.OptWebhookFailure{}
	}
	return externalapi.NewOptWebhookFailure(externalapi.WebhookFailure{
		ConsecutiveFailures: narrow.Int32(*h.ConsecutiveFailures),
		Since:               externalapi.Timestamp(*h.FailingSince),
		LastFailureAt:       externalapi.Timestamp(*h.LastFailureAt),
		Kind:                c.WebhookErrorKindToResource(*h.LastErrorKind),
		Status:              optInt32(h.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(c Converter, v *webhookendpoint.DisabledReason) externalapi.OptWebhookDisabledReason {
	return convert.OptVia[externalapi.OptWebhookDisabledReason](v, c.WebhookDisabledReasonToResource)
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) externalapi.OptNilString {
	return convert.OptNonZero[externalapi.OptNilString](s)
}

// omitEmptyConfigurationSet is an SES Integration's configuration set, absent when it names none.
func omitEmptyConfigurationSet(s string) externalapi.OptNilSesConfigurationSetName {
	return convert.OptNonZero[externalapi.OptNilSesConfigurationSetName](externalapi.SesConfigurationSetName(s))
}

func smtpConfigKind(integrations.RedactedSMTP) externalapi.SmtpConfigKind {
	return externalapi.SmtpConfigKindSMTP
}

func sesConfigKind(integrations.RedactedSES) externalapi.SesConfigKind {
	return externalapi.SesConfigKindSes
}

func twilioConfigKind(integrations.RedactedTwilio) externalapi.TwilioConfigKind {
	return externalapi.TwilioConfigKindTwilio
}

// twilioAPIKeySID is the API key messages are sent with, null when they are sent with the Auth Token.
func twilioAPIKeySID(c integrations.RedactedTwilio) externalapi.NilString {
	if c.APIKeySID == "" {
		return externalapi.NilString{Null: true}
	}
	return externalapi.NewNilString(c.APIKeySID)
}

func telegramConfigKind(integrations.RedactedTelegram) externalapi.TelegramConfigKind {
	return externalapi.TelegramConfigKindTelegram
}

// sendingDomainVerified derives the verified flag from the verified and lost moments.
func sendingDomainVerified(d ent.SendingDomain) bool { return dkimstate.Is(&d) }

// webhookEnabled derives the enabled flag: an endpoint is enabled until it has a disabled moment.
func webhookEnabled(e ent.WebhookEndpoint) bool { return e.DisabledAt == nil }

// ruleTriggerEvents names a Rule version's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(v *ent.RuleVersion) []string { return rules.Triggers(v) }

// ruleVersionOrNull is a version of the Rule, or null when it has none.
func ruleVersionOrNull(c Converter, v *ent.RuleVersion) (externalapi.NilRuleVersion, error) {
	if v == nil {
		return externalapi.NilRuleVersion{Null: true}, nil
	}
	version, err := c.RuleVersionToResource(v)
	if err != nil {
		return externalapi.NilRuleVersion{}, err
	}
	return externalapi.NewNilRuleVersion(version), nil
}

// optNonEmpty is a text that is absent when empty.
func optNonEmpty(s string) externalapi.OptString {
	return convert.OptNonZero[externalapi.OptString](s)
}

// ruleActions decodes a Rule's stored Actions into the typed DTO. Stored text the module cannot
// decode (only ever written by the module itself) yields none, and is logged: a converter has
// no way to fail a response, and no caller can act on it.
func ruleActions(c Converter, actions []ruleaction.Action) []externalapi.RuleAction {
	return lo.Map(actions, func(a ruleaction.Action, _ int) externalapi.RuleAction {
		out := externalapi.RuleAction{Type: c.RuleActionTypeToResource(a.Type), Tag: optNonEmpty(a.Tag)}
		if a.Type == ruleaction.Enroll {
			out.AutomationId = externalapi.NewOptEntityId(externalapi.EntityId(a.AutomationID))
		}
		if a.Type == ruleaction.HTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = optNonEmpty(a.ID)
			out.Body = optNonEmpty(a.Body)
			out.HasSecret = externalapi.NewOptBool(a.Secret != "")
			out.Headers = lo.Map(a.Headers, func(h httpheader.Header, _ int) externalapi.AutomationHttpHeader {
				return externalapi.AutomationHttpHeader{Name: h.Name}
			})
		}
		return out
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO; stored text the module
// cannot decode yields none and is logged, as ruleActions does.
func ruleResults(c Converter, stored string) []externalapi.RuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) externalapi.RuleActionResult {
		return externalapi.RuleActionResult{
			Type:   c.RuleActionTypeToResource(r.Type),
			Status: c.RuleActionStatusToResource(r.Status),
			Detail: optNonEmpty(r.Detail),
		}
	})
}

// optNilSmsConfirmation is absent exactly when the phone has no SMS Confirmation.
func optNilSmsConfirmation(c Converter, v *ent.Confirmation) externalapi.OptNilSmsConfirmation {
	if v == nil {
		return externalapi.OptNilSmsConfirmation{}
	}
	return externalapi.NewOptNilSmsConfirmation(c.SmsConfirmationToResource(v))
}

// optNilSmsOptOut is absent exactly when the phone has no SMS opt-out.
func optNilSmsOptOut(c Converter, v *confirmations.SMSOptOut) externalapi.OptNilSmsOptOut {
	if v == nil {
		return externalapi.OptNilSmsOptOut{}
	}
	return externalapi.NewOptNilSmsOptOut(c.SmsOptOutToResource(v))
}

func optNilOptOutVia(c Converter, v *unsubscribe.Via) externalapi.OptNilOptOutVia {
	return convert.OptVia[externalapi.OptNilOptOutVia](v, c.OptOutViaToResource)
}

func optNilEngagementTrackingProvenance(c Converter, v *engagementtracking.Provenance) externalapi.OptNilEngagementTrackingProvenance {
	return convert.OptVia[externalapi.OptNilEngagementTrackingProvenance](v, c.EngagementTrackingProvenanceToResource)
}

func optNilHoldReason(c Converter, v *holdreason.Reason) externalapi.OptNilHoldReason {
	return convert.OptVia[externalapi.OptNilHoldReason](v, c.HoldReasonToResource)
}

// schemaExample is the agent schema's example document as the contract's opaque object.
func schemaExample(doc json.RawMessage) externalapi.EditorDocument {
	return convert.DocObject[externalapi.EditorDocument](doc)
}

// customFieldConditionTests is what a Condition may ask of a Custom field of the type: the
// Condition's table, listed so a client offers what the server takes.
func customFieldConditionTests(c Converter, t customfield.Type) externalapi.CustomFieldConditionTests {
	return c.ConditionTestsToResource(condition.CustomFieldTests(t))
}

// optNilSendCap is null exactly when no Send cap applies.
func optNilSendCap(c Converter, v *providerlimit.SendCapStatus) externalapi.OptNilSendCap {
	return convert.NilVia[externalapi.OptNilSendCap](v, c.SendCapToResource)
}

// revision is a row's revision (ADR 0119) as the contract carries it.
func revision(v int64) externalapi.Revision {
	return externalapi.Revision(v)
}

// ownershipRecordType is the type of a Tracking domain's ownership record (ADR 0173).
func ownershipRecordType(trackingdomains.OwnershipRecord) externalapi.DnsRecordType {
	return externalapi.DnsRecordTypeTXT
}
