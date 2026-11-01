package events

import "context"

// The System events of what happens to a Contact rather than what it does (#897): its Tags, its
// Custom fields and its Enrollments. Each is published in the transaction of the change it
// records, and only for a real change, so a Trigger, a Rule or a Segment that names one sees each
// change once. All of them are analytical: Retention deletes them by age (ADR 0019).

// ContactTagChange is a Tag applied to a Contact that did not have it (contact.tagged) or removed
// from one that had it (contact.untagged). Tag is the Tag's name.
type ContactTagChange struct {
	Action      string `json:"action"` // contact.tagged|contact.untagged
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email,omitempty"`
	Tag         string `json:"tag"`
}

func (e *ContactTagChange) EventName() string { return e.Action }
func (*ContactTagChange) EventVersion() int   { return 1 }
func (e *ContactTagChange) Workspace() int64  { return e.WorkspaceID }
func (e *ContactTagChange) Project() Projection {
	return Projection{
		Subject: e.Email, Action: e.Action, Email: e.Email, ContactID: e.ContactID,
		Properties: map[string]any{"tag": e.Tag},
	}
}

// ContactFieldChanged is a Contact's Custom field taking a value it did not hold: set where it had
// none, changed, or cleared. Value is the new value; a cleared field has none, and the old value is
// never recorded.
type ContactFieldChanged struct {
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email,omitempty"`
	Key         string `json:"key"`
	Value       any    `json:"value,omitempty"`
}

func (*ContactFieldChanged) EventName() string  { return NameContactFieldChanged }
func (*ContactFieldChanged) EventVersion() int  { return 1 }
func (e *ContactFieldChanged) Workspace() int64 { return e.WorkspaceID }
func (e *ContactFieldChanged) Project() Projection {
	props := map[string]any{"key": e.Key}
	if e.Value != nil {
		props["value"] = e.Value
	}
	return Projection{
		Subject: e.Email, Action: NameContactFieldChanged, Email: e.Email, ContactID: e.ContactID,
		Properties: props,
	}
}

// EnrollmentChange is a Contact's Enrollment in an Automation beginning (automation.enrolled) or
// ending completed, at its goal or exited. The Automation is the Event's automation_id, and its
// `automationId` property too, which a Rule's trigger leaf and the feeds read.
type EnrollmentChange struct {
	Action       string `json:"action"` // automation.enrolled|completed|goal_reached|exited
	WorkspaceID  int64  `json:"workspaceId"`
	ContactID    int64  `json:"contactId"`
	Email        string `json:"email,omitempty"`
	AutomationID int64  `json:"automationId"`
}

func (e *EnrollmentChange) EventName() string { return e.Action }
func (*EnrollmentChange) EventVersion() int   { return 1 }
func (e *EnrollmentChange) Workspace() int64  { return e.WorkspaceID }
func (e *EnrollmentChange) Project() Projection {
	return Projection{
		Subject: e.Email, Action: e.Action, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, Properties: map[string]any{"automationId": e.AutomationID},
	}
}

type causeKey struct{}

// CausedBy returns ctx for the work of a Rule's Action: an event published in it carries cause
// as its envelope's CausedBy, and the Rules consumer skips it, so Rules never chain (ADR 0054).
// It is not carried into the jobs the work queues: the steps of an Enrollment a Rule made are the
// Automation's, not the Rule's.
func CausedBy(ctx context.Context, cause string) context.Context {
	return context.WithValue(ctx, causeKey{}, cause)
}

// causeOf is the cause ctx carries, "" outside the work of a Rule's Action.
func causeOf(ctx context.Context) string {
	cause, _ := ctx.Value(causeKey{}).(string)
	return cause
}
