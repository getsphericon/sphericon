// Package inuse is the one shape of a refused removal (ADR 0049): a row that live work still
// uses is not removed, and the refusal names the users. Each domain module returns an *Error
// from its delete; each API surface renders it as its contract's InUseProblem (409, `usedBy`).
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
	// Rule is an active Rule with an Action that uses the row.
	Rule Kind = "rule"
	// Enrollment stands for the Enrollments of an Automation: one User naming the
	// Automation, with their Count.
	Enrollment Kind = "enrollment"
)

// Subject is the row a refused removal was for.
type Subject string

// The rows a removal can be refused for; the text reads "<subject> is in use by ...".
const (
	SubjectSegment       Subject = "segment"
	SubjectTag           Subject = "tag"
	SubjectIntegration   Subject = "integration"
	SubjectSendingDomain Subject = "sending domain"
	SubjectAutomation    Subject = "automation"
)

// User is one row that uses the row a removal was refused for.
type User struct {
	Kind Kind
	ID   int64
	Name string
	// Count is how many rows an Enrollment User stands for; every other kind is one row and
	// leaves it zero.
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

// Error is a removal refused because Users still use the row. Callers match it with As.
type Error struct {
	// What is the removed row, for the message.
	What  Subject
	Users []User
}

func (e *Error) Error() string {
	names := make([]string, len(e.Users))
	for i, u := range e.Users {
		names[i] = fmt.Sprintf("%s %q", u.Kind, u.Name)
		if u.Kind == Enrollment {
			names[i] = fmt.Sprintf("%d %s of %q", u.Count, lo.Ternary(u.Count == 1, "enrollment", "enrollments"), u.Name)
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
