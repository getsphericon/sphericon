package contacts_test

import (
	"context"
	"errors"
	"maps"
	"net"
	"sync"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/addresscheck"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestUpsertBatchRunsEachItemIndependentlyAndInOrder(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus)

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, []contacts.IntakeInput{
		{Email: new("batch.new@example.com"), CustomFields: map[string]any{"source": "import"}},
		{Email: lo.ToPtr(fixtures.ContactAliceEmail), FirstName: patch.Set("Alicia")},
		{FirstName: patch.Set("No identity")},
	})
	require.Len(t, out.Items, 3)

	require.NoError(t, out.Items[0].Err)
	assert.True(t, out.Items[0].Value.Created)
	assert.Equal(t, "import", out.Items[0].Value.Contact.CustomFields["source"])
	assert.Equal(t, 1, createdEvents(t, env, out.Items[0].Value.Contact.ID))

	require.NoError(t, out.Items[1].Err)
	assert.False(t, out.Items[1].Value.Created)
	assert.EqualValues(t, fixtures.ContactAliceID, out.Items[1].Value.Contact.ID)
	assert.Equal(t, "Alicia", lo.FromPtr(out.Items[1].Value.Contact.FirstName), "an asserted attribute overwrites")

	require.ErrorIs(t, out.Items[2].Err, contacts.ErrIdentityRequired, "a failing item does not stop the batch")
}

// mailZones are the domains of the address check tests: one with a mail exchanger, one with a
// null MX, a disposable provider's and one whose server fails; any other name is NXDOMAIN.
func mailZones(t *testing.T) *countingResolver {
	t.Helper()
	return &countingResolver{Resolver: dnstest.Resolver(t, map[string]mockdns.Zone{
		"mx.test.":        {MX: []net.MX{{Host: "mail.mx.test.", Pref: 10}}},
		"nullmx.test.":    {MX: []net.MX{{Host: ".", Pref: 0}}},
		"mailinator.com.": {MX: []net.MX{{Host: "mail.mailinator.com.", Pref: 10}}},
		"broken.test.":    {Err: errors.New("server failure")},
	})}
}

func TestUpsertBatchRefusesANewAddressAtADomainWithoutMail(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus, contacts.WithMailResolver(mailZones(t)))

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{
		{Email: new("ann@nullmx.test")},
		{Email: new("bob@missing.test")},
		{Email: new("cat@mx.test")},
	})
	require.Len(t, out.Items, 3)

	require.ErrorIs(t, out.Items[0].Err, contacts.ErrEmailDomainNoMail, "a null MX")
	require.ErrorIs(t, out.Items[1].Err, contacts.ErrEmailDomainNoMail, "NXDOMAIN")
	require.NoError(t, out.Items[2].Err)
	assert.True(t, out.Items[2].Value.Created)
	for _, address := range []string{"ann@nullmx.test", "bob@missing.test"} {
		held, err := env.DB.Contact.Query().Where(contact.EmailEQ(address)).Exist(context.Background())
		require.NoError(t, err)
		assert.False(t, held, "%s was not stored", address)
	}
}

func TestUpsertBatchUpdatesAnAddressTheWorkspaceHoldsWhateverItsDomain(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus, contacts.WithMailResolver(mailZones(t)))

	// example.com is outside the zones, so it is NXDOMAIN here; Alice holds the address already.
	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{
		{Email: lo.ToPtr(fixtures.ContactAliceEmail), FirstName: patch.Set("Alicia")},
	})

	require.NoError(t, out.Items[0].Err)
	assert.EqualValues(t, fixtures.ContactAliceID, out.Items[0].Value.Contact.ID)
	assert.Equal(t, "Alicia", lo.FromPtr(out.Items[0].Value.Contact.FirstName))
}

func TestUpsertBatchAcceptsAnAddressWhoseDomainDNSDoesNotAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus, contacts.WithMailResolver(mailZones(t)))

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{
		{Email: new("dan@broken.test")},
	})

	require.NoError(t, out.Items[0].Err)
	assert.True(t, out.Items[0].Value.Created)
}

func TestUpsertBatchFlagsDisposableAndRoleAddresses(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus, contacts.WithMailResolver(mailZones(t)))

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{
		{Email: new("eve@mailinator.com")},
		{Email: new("info@mx.test")},
		{Email: new("fay@mx.test")},
	})

	require.NoError(t, out.Items[0].Err)
	assert.Equal(t, []addresscheck.Flag{addresscheck.Disposable}, out.Items[0].Value.Flags)
	require.NoError(t, out.Items[1].Err)
	assert.Equal(t, []addresscheck.Flag{addresscheck.Role}, out.Items[1].Value.Flags)
	require.NoError(t, out.Items[2].Err)
	assert.Empty(t, out.Items[2].Value.Flags)
}

func TestUpsertBatchLooksUpEachDomainOnce(t *testing.T) {
	env := testhelper.Setup(t)
	zones := mailZones(t)
	intake := contacts.New(env.Bus, contacts.WithMailResolver(zones))

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{
		{Email: new("gus@mx.test")},
		{Email: new("Hal@MX.test")},
		{Email: new("ivy@mx.test")},
	})

	for _, item := range out.Items {
		require.NoError(t, item.Err)
	}
	assert.Equal(t, map[string]int{"mx.test": 1}, zones.mxLookups())
}

// countingResolver counts the MX lookups of each name it passes on.
type countingResolver struct {
	*net.Resolver

	mu    sync.Mutex
	names map[string]int
}

func (r *countingResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	r.mu.Lock()
	if r.names == nil {
		r.names = map[string]int{}
	}
	r.names[name]++
	r.mu.Unlock()
	return r.Resolver.LookupMX(ctx, name)
}

func (r *countingResolver) mxLookups() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.names)
}
