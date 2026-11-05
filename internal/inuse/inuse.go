// Package inuse is the one shape of a refused removal (ADR 0049): a row that live work still
// uses is not removed, and the refusal names the users. Each domain module returns an *Error
// from its delete; internal/problem renders it as the contracts' Problem (409, `usedBy`) on every
// surface.
package inuse

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
)

// Kind is what a user of a row is.
type Kind string

// The kinds of live user a removal can find; each has a value of the contract's UsedByKind.
const (
	// Broadcast is a Broadcast that has not finished sending.
	Broadcast Kind = "broadcast"
	// Automation is an Automation: one whose steps use the row, or an active one.
	Automation Kind = "automation"
	// Segment is a Segment whose rule names the row.
	Segment Kind = "segment"
	// Rule is a kept Rule that uses the row in its live version or its draft: through a Condition
	// leaf or an Action.
	Rule Kind = "rule"
	// Enrollment stands for the active Enrollments of an Automation: one User naming the
	// Automation, with their Count.
	Enrollment Kind = "enrollment"
)

// Subject is the row a refused removal was for.
type Subject string

// The rows a removal can be refused for; the text reads "<subject> is in use by ...".
const (
	SubjectTag           Subject = "tag"
	SubjectIntegration   Subject = "integration"
	SubjectSendingDomain Subject = "sending domain"
	SubjectAutomation    Subject = "automation"
	// SubjectCustomField is a Custom field whose type change was refused: what uses it is a
	// Condition leaf the new type does not take.
	SubjectCustomField Subject = "custom field"
)

// User is one row that uses the row a removal was refused for.
type User struct {
	Kind Kind
	ID   int64
	Name string
	// Count is how many rows an Enrollment User stands for; every other kind is one row and leaves
	// it zero.
	Count int
}

// Enrollments is the User that stands for count Enrollments of the Automation.
func Enrollments(automationID int64, name string, count int) User {
	return User{Kind: Enrollment, ID: automationID, Name: name, Count: count}
}

// Broadcasts maps Broadcasts to their Users, in the order given.
func Broadcasts(rows []*ent.Broadcast) []User {
	return lo.Map(rows, func(b *ent.Broadcast, _ int) User {
		return User{Kind: Broadcast, ID: b.ID, Name: b.Name}
	})
}

// Rules maps Rules to their Users, in the order given.
func Rules(rows []*ent.Rule) []User {
	return lo.Map(rows, func(r *ent.Rule, _ int) User {
		return User{Kind: Rule, ID: r.ID, Name: r.Name}
	})
}

// Error is a removal refused because Users still use the row, or a change refused because they
// use it in a way the change would break (a Custom field's type). Callers match it with As.
type Error struct {
	// What is the row removed or changed, for the message.
	What  Subject
	Users []User
}

func (e *Error) Error() string {
	names := make([]string, len(e.Users))
	for i, u := range e.Users {
		names[i] = fmt.Sprintf("%s %q", u.Kind, u.Name)
		if u.Kind == Enrollment {
			// A counted User: "2 enrollments of "name"".
			names[i] = fmt.Sprintf("%d %s of %q", u.Count, lo.Ternary(u.Count == 1, string(u.Kind), string(u.Kind)+"s"), u.Name)
		}
	}
	return fmt.Sprintf("%s is in use by %s", e.What, strings.Join(names, ", "))
}

// As reports the refusal an error carries, if it is one.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
