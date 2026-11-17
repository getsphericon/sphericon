package events_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

// notAContact are the events whose address names a User or an invitee of the Workspace, never a
// Contact: Erasure leaves their jobs queued (PurgeEventJobs).
var notAContact = []string{
	events.NameInvitationCreated, events.NameUserAddressClaimed, events.NameUserDeleted,
	events.NameUserDeletionRequested, events.NameUserEmailChangeRequested, events.NameUserIdentityLinked,
	events.NameUserPasskeyAdded, events.NameUserPasskeyRemoved, events.NameUserPasswordResetRequested,
	events.NameUserSecondFactorReset, events.NameUserSignedUp, events.NameUserVerificationRequested,
}

// carriesAPerson reports whether the events of name have a field that names a person: an id of a
// Contact or a destination.
func carriesAPerson(name string) bool {
	ev := lo.Must(events.Decode(events.Envelope{Name: name, Data: []byte("{}")}))
	return slices.ContainsFunc(reflect.VisibleFields(reflect.TypeOf(ev).Elem()), func(f reflect.StructField) bool {
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		return slices.Contains([]string{"contactId", "email", "phone", "destination"}, key)
	})
}

// Every event that carries a person says whether it is a Contact's, so a new one is purged on
// Erasure or left alone on purpose, never by the name of a JSON field.
func TestEveryEventWithAPersonSaysWhetherItIsAContacts(t *testing.T) {
	carriers := lo.Filter(events.Names(), func(name string, _ int) bool { return carriesAPerson(name) })
	contacts := lo.Filter(carriers, func(name string, _ int) bool { return events.NamesContact(name) })

	assert.ElementsMatch(t, carriers, append(contacts, notAContact...),
		"each event with a person names a Contact (Projected or namesContact) or is listed in notAContact, never both")
}
