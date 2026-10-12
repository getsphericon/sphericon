package usage_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent/contact"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// clock is the harness clock a test moves: the contacts peak is kept as Contacts change,
// so it needs the time of the change.
type clock struct{ t atomic.Pointer[time.Time] }

func newClock(t time.Time) *clock {
	c := &clock{}
	c.set(t)
	return c
}
func (c *clock) set(t time.Time) { c.t.Store(&t) }
func (c *clock) now() time.Time  { return *c.t.Load() }

// contactsEnv is a licensed instance on a moved clock, and the number of identified
// Contacts Acme has as the fixtures leave them.
func contactsEnv(t *testing.T, now time.Time) (*testhelper.TestEnv, *clock, int64) {
	t.Helper()
	c := newClock(now)
	env := testhelper.Setup(t, testhelper.WithClock(c.now))
	base, err := env.DB.Scoped(fixtures.AcmeID).Contact().Query().
		Where(contact.EmailNotNil()).Count(t.Context())
	require.NoError(t, err)
	return env, c, int64(base)
}

// addContact creates a Contact of Acme: identified with the email, or an anonymous
// tracker Contact (no email Destination) when it is empty.
func addContact(t *testing.T, env *testhelper.TestEnv, email string) int64 {
	t.Helper()
	c := env.DB.Scoped(fixtures.AcmeID).Contact().Create()
	if email != "" {
		c.SetEmail(email)
	} else {
		c.SetSubjectID("anonymous-" + t.Name())
	}
	created, err := c.Save(t.Context())
	require.NoError(t, err)
	return created.ID
}

func deleteContact(t *testing.T, env *testhelper.TestEnv, id int64) {
	t.Helper()
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Contact().DeleteOneID(id).Exec(t.Context()))
}

func refreshAt(t *testing.T, env *testhelper.TestEnv, now time.Time) {
	t.Helper()
	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureOperator), now))
}

func TestContactsIsThePeakReachedAndSurvivesLaterDeletions(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	a := addContact(t, env, "peak-a@example.com")
	b := addContact(t, env, "peak-b@example.com")
	c := addContact(t, env, "peak-c@example.com")
	deleteContact(t, env, a)
	deleteContact(t, env, b)
	deleteContact(t, env, c)
	deleteContact(t, env, fixtures.ContactBobID)

	refreshAt(t, env, now.Add(time.Hour))

	f := figureOf(t, env, feb, usage.Contacts)
	assert.Equal(t, base+3, f.Value, "the peak, not what is left")
	assert.False(t, f.Final)
}

func TestContactsDoesNotCountAnAnonymousContact(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	addContact(t, env, "")

	refreshAt(t, env, now)

	assert.Equal(t, base, figureOf(t, env, feb, usage.Contacts).Value)
}

func TestContactsDoesNotCountAnErasedContact(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	require.NoError(t, erasure.New(env.Bus).Erase(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		erasure.ByContactID(fixtures.ContactAliceID), erasure.Operator{Kind: erasure.OperatorAPIToken, ID: 1}))
	addContact(t, env, "after-erasure@example.com")

	refreshAt(t, env, now)

	assert.Equal(t, base, figureOf(t, env, feb, usage.Contacts).Value,
		"the erased Contact's place was taken, so the peak did not grow")
}

func TestContactsPeakGrowsWhenAnAnonymousContactGetsAnEmail(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	id := addContact(t, env, "")
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Contact().UpdateOneID(id).
		SetEmail("identified-later@example.com").Exec(t.Context()))
	deleteContact(t, env, id)

	refreshAt(t, env, now)

	assert.Equal(t, base+1, figureOf(t, env, feb, usage.Contacts).Value)
}

func TestContactsPeakIsFrozenAtTheCloseAndTheNextPeriodStartsFromWhatRemains(t *testing.T) {
	env, clk, base := contactsEnv(t, at(2027, time.January, 31, 22))
	id := addContact(t, env, "january-peak@example.com")
	deleteContact(t, env, id)

	clk.set(feb.Add(time.Minute))
	refreshAt(t, env, feb.Add(time.Minute))
	addContact(t, env, "february@example.com")
	addContact(t, env, "february-2@example.com")
	refreshAt(t, env, feb.Add(time.Hour))

	january := figureOf(t, env, jan, usage.Contacts)
	assert.Equal(t, base+1, january.Value)
	assert.True(t, january.Final)
	assert.Equal(t, base+2, figureOf(t, env, feb, usage.Contacts).Value, "February starts from the Contacts left in January")
}

func TestContactsIsNotMaintainedWithoutTheOperatorLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	addContact(t, env, "unlicensed@example.com")

	n, err := env.DB.Scoped(fixtures.AcmeID).Usage().Query().Where(entusage.Metric(string(usage.Contacts))).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}
