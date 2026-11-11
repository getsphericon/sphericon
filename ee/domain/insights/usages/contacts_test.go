package usages_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
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
	// test writes: the contacts-peak hook fires on this Contact write
	c := env.FeedDB.Scoped(fixtures.AcmeID).Contact().Create()
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
	require.NoError(t, env.FeedDB.Scoped(fixtures.AcmeID).Contact().DeleteOneID(id).Exec(t.Context()))
}

func refreshAt(t *testing.T, env *testhelper.TestEnv, now time.Time) {
	t.Helper()
	require.NoError(t, usages.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), now))
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

	f := figureOf(t, env, feb, usages.Contacts)
	assert.Equal(t, base+3, f.Value, "the peak, not what is left")
	assert.False(t, f.Final)
}

func TestContactsDoesNotCountAnAnonymousContact(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	addContact(t, env, "")

	refreshAt(t, env, now)

	assert.Equal(t, base, figureOf(t, env, feb, usages.Contacts).Value)
}

func TestContactsDoesNotCountAnErasedContact(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	require.NoError(t, contacterasure.New(env.Bus, time.Now).Erase(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		contacterasure.Identifier{ContactID: fixtures.ContactAliceID}, contacterasure.Requester{Kind: contacterasure.RequesterAPIToken, ID: 1}))
	addContact(t, env, "after-erasure@example.com")

	refreshAt(t, env, now)

	assert.Equal(t, base, figureOf(t, env, feb, usages.Contacts).Value,
		"the erased Contact's place was taken, so the peak did not grow")
}

func TestContactsPeakGrowsWhenAnAnonymousContactGetsAnEmail(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	id := addContact(t, env, "")
	require.NoError(t, env.FeedDB.Scoped(fixtures.AcmeID).Contact().UpdateOneID(id).
		SetEmail("identified-later@example.com").Exec(t.Context()))
	deleteContact(t, env, id)

	refreshAt(t, env, now)

	assert.Equal(t, base+1, figureOf(t, env, feb, usages.Contacts).Value)
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

	january := figureOf(t, env, jan, usages.Contacts)
	assert.Equal(t, base+1, january.Value)
	assert.True(t, january.Final)
	assert.Equal(t, base+2, figureOf(t, env, feb, usages.Contacts).Value, "February starts from the Contacts left in January")
}

// The peak moves in the transaction of the Contact change, so a client outside a transaction
// refuses a change that could move it rather than leave the peak behind.
func TestAPlainClientRefusesAContactChangeThatCanMoveThePeak(t *testing.T) {
	env, _, _ := contactsEnv(t, at(2027, time.February, 10, 12))
	acme := env.DB.Scoped(fixtures.AcmeID)

	// test writes: the guard refuses this Contact write before it reaches the table
	require.ErrorIs(t, acme.Contact().Create().SetEmail("plain@example.com").Exec(t.Context()), usages.ErrNeedsTransaction)
	require.ErrorIs(t, acme.Contact().UpdateOneID(fixtures.ContactAliceID).SetEmail("alice-new@example.com").Exec(t.Context()), usages.ErrNeedsTransaction)
	require.ErrorIs(t, acme.Contact().UpdateOneID(fixtures.ContactAliceID).ClearEmail().Exec(t.Context()), usages.ErrNeedsTransaction)
	require.ErrorIs(t, acme.Contact().DeleteOneID(fixtures.ContactBobID).Exec(t.Context()), usages.ErrNeedsTransaction)
}

func TestAPlainClientLetsAContactChangeThatCannotMoveThePeakThrough(t *testing.T) {
	env, _, _ := contactsEnv(t, at(2027, time.February, 10, 12))
	acme := env.DB.Scoped(fixtures.AcmeID)

	// test writes: an anonymous Contact and a name change, which the guard lets through
	require.NoError(t, acme.Contact().Create().SetSubjectID("anonymous-"+t.Name()).Exec(t.Context()))
	require.NoError(t, acme.Contact().UpdateOneID(fixtures.ContactAliceID).SetFirstName("Alicia").Exec(t.Context()))
}

func TestAPlainClientRefusesNoContactChangeWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	// test writes: without the license there is no peak, so the guard lets the write through
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Contact().Create().SetEmail("unlicensed@example.com").Exec(t.Context()))
}

func TestContactsIsNotMaintainedWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	addContact(t, env, "unlicensed@example.com")

	n, err := env.DB.Scoped(fixtures.AcmeID).Usage().Query().Where(entusage.Metric(string(usages.Contacts))).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestContactsPeakOfABulkImportIsTheNumberReachedEvenWhenSomeAreDeleted(t *testing.T) {
	now := at(2027, time.February, 10, 12)
	env, _, base := contactsEnv(t, now)
	const imported = 30
	ids := make([]int64, 0, imported)
	for i := range imported {
		ids = append(ids, addContact(t, env, fmt.Sprintf("bulk-%d@example.com", i)))
	}
	for _, id := range ids[:10] {
		deleteContact(t, env, id)
	}

	refreshAt(t, env, now.Add(time.Hour))

	assert.Equal(t, base+imported, figureOf(t, env, feb, usages.Contacts).Value)
}

func TestContactsPeakIsNotKeptForAWorkspaceInItsDeletionWindow(t *testing.T) {
	now := time.Date(2027, time.March, 10, 12, 0, 0, 0, time.UTC)
	env, _, _ := contactsEnv(t, now)
	s := env.DB.Scoped(fixtures.BluthID)
	before, err := s.Usage().Query().Count(t.Context())
	require.NoError(t, err)

	// test writes: the contacts-peak hook fires on this Contact write
	require.NoError(t, env.FeedDB.Scoped(fixtures.BluthID).Contact().Create().SetEmail("late@bluth.test").Exec(t.Context()))

	after, err := s.Usage().Query().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// Two transactions that add a Contact at once both count in the peak: each counts and raises it in
// turn, behind the Workspace's contacts-peak lock, so the second counts the first's committed row.
func TestContactsPeakCountsContactsAddedAtOnce(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	base, err := race.Scope.Contact().Query().Where(contact.EmailNotNil()).Count(t.Context())
	require.NoError(t, err)
	add := func(email string) testhelper.Writer {
		return testhelper.Writer{Name: email, Run: func() error {
			return race.Bus.WithinScopedTx(t.Context(), race.Scope, func(ts *ent.Scoped, _ events.Publisher) error {
				// test writes: the contacts-peak hook fires on this Contact write
				return ts.Contact().Create().SetEmail(email).Exec(t.Context())
			})
		}}
	}

	errs := race.QueueBehindKey(t, postgres.WorkspaceKey(usages.PeakLock, fixtures.AcmeID),
		add("first@example.com"), add("second@example.com"))

	require.NoError(t, errs["first@example.com"])
	require.NoError(t, errs["second@example.com"])
	peak, err := race.Scope.Usage().Query().Where(entusage.Metric(string(usages.Contacts))).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(base+2), peak.Value)
}

// A transaction that adds a Contact takes the contacts-peak lock only right before it commits,
// after its row locks. Here B holds Bob's row and adds a Contact while A, which added one first,
// waits for Bob's row: had A taken the lock at its write, B would wait for A and A for B.
func TestContactsPeakLockWaitsForNoRowLock(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	base, err := race.Scope.Contact().Query().Where(contact.EmailNotNil()).Count(t.Context())
	require.NoError(t, err)
	aAdded, bAdded := make(chan struct{}), make(chan struct{})

	a := goWrite(func() error {
		return race.Bus.WithinScopedTx(t.Context(), race.Scope, func(ts *ent.Scoped, _ events.Publisher) error {
			return addThenRename(t, ts, "a@example.com", aAdded, bAdded)
		})
	})
	b := goWrite(func() error {
		return race.Bus.WithinScopedTx(t.Context(), race.Scope, func(ts *ent.Scoped, _ events.Publisher) error {
			return renameThenAdd(t, ts, "b@example.com", aAdded, bAdded)
		})
	})

	require.NoError(t, testhelper.Receive(t, b, "B commits"))
	require.NoError(t, testhelper.Receive(t, a, "A commits once B let Bob go"))
	peak, err := race.Scope.Usage().Query().Where(entusage.Metric(string(usages.Contacts))).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(base+2), peak.Value)
}

// addThenRename adds a Contact, lets the other side know, and once it added its own renames Bob,
// whose row the other side holds.
func addThenRename(t *testing.T, ts *ent.Scoped, email string, added chan<- struct{}, other <-chan struct{}) error {
	t.Helper()
	// test writes: the contacts-peak hook fires on this Contact write
	if err := ts.Contact().Create().SetEmail(email).Exec(t.Context()); err != nil {
		return err
	}
	close(added)
	if err := await(other); err != nil {
		return err
	}
	// test writes: Bob's row, which the other side holds until it commits
	return ts.Contact().UpdateOneID(fixtures.ContactBobID).SetFirstName("A").Exec(t.Context())
}

// renameThenAdd renames Bob, holding his row, and adds a Contact once the other side added one.
func renameThenAdd(t *testing.T, ts *ent.Scoped, email string, other <-chan struct{}, added chan<- struct{}) error {
	t.Helper()
	// test writes: takes Bob's row before the other side asks for it
	if err := ts.Contact().UpdateOneID(fixtures.ContactBobID).SetFirstName("B").Exec(t.Context()); err != nil {
		return err
	}
	if err := await(other); err != nil {
		return err
	}
	// test writes: the contacts-peak hook fires on this Contact write
	if err := ts.Contact().Create().SetEmail(email).Exec(t.Context()); err != nil {
		return err
	}
	close(added)
	return nil
}

// await waits for ch to close, or fails after a while: a side that waits for the other forever is
// the two waiting for each other.
func await(ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("the other side never got there")
	}
}

// goWrite runs write on a goroutine of its own and hands its error over.
func goWrite(write func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- write() }()
	return done
}
