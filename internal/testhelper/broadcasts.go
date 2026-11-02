package testhelper

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// SiteEmail is the email content of a Broadcast write on the site API, its Channel set, so a test
// writes only the fields it is about instead of the generated union.
func SiteEmail(c siteapi.EmailBroadcastContentInput) siteapi.BroadcastContentInput {
	c.Channel = siteapi.EmailBroadcastContentInputChannelEmail
	return siteapi.BroadcastContentInput{OneOf: siteapi.NewEmailBroadcastContentInputBroadcastContentInputSum(c)}
}

// SiteEmailEdit is SiteEmail as the content of a site API update.
func SiteEmailEdit(c siteapi.EmailBroadcastContentInput) siteapi.OptBroadcastContentInput {
	return siteapi.NewOptBroadcastContentInput(SiteEmail(c))
}

// ExternalEmail is SiteEmail for the external API.
func ExternalEmail(c externalapi.EmailBroadcastContentInput) externalapi.BroadcastContentInput {
	c.Channel = externalapi.EmailBroadcastContentInputChannelEmail
	return externalapi.BroadcastContentInput{OneOf: externalapi.NewEmailBroadcastContentInputBroadcastContentInputSum(c)}
}

// ExternalEmailEdit is ExternalEmail as the content of an external API update.
func ExternalEmailEdit(c externalapi.EmailBroadcastContentInput) externalapi.OptBroadcastContentInput {
	return externalapi.NewOptBroadcastContentInput(ExternalEmail(c))
}

// SiteEmailOf is the email content of a site API Broadcast resource; the zero value when the
// Broadcast is on another Channel.
func SiteEmailOf(b *siteapi.SiteBroadcastResource) siteapi.EmailBroadcastContent {
	return b.Content.OneOf.EmailBroadcastContent
}

// ExternalEmailOf is SiteEmailOf for the external API.
func ExternalEmailOf(b *externalapi.BroadcastResource) externalapi.EmailBroadcastContent {
	return b.Content.OneOf.EmailBroadcastContent
}

// PlaceRecipient moves the parked recipient of a Contact (Alice, Bob or Carol, parked on the archived
// Broadcast in the fixtures) onto the Broadcast, with its message if it has one, and returns the
// recipient's id. A test that needs a Broadcast with exactly the recipients it names places them.
func (env *TestEnv) PlaceRecipient(ctx context.Context, broadcastID, contactID int64) int64 {
	s := env.DB.Scoped(fixtures.AcmeID)
	id := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastArchivedID), broadcastrecipient.ContactID(contactID)).
		OnlyX(ctx).ID
	if err := s.BroadcastRecipient().UpdateOneID(id).SetBroadcastID(broadcastID).Exec(ctx); err != nil {
		panic(err)
	}
	if err := s.OutboundMessage().Update().Where(outboundmessage.BroadcastRecipientID(id)).SetBroadcastID(broadcastID).Exec(ctx); err != nil {
		panic(err)
	}
	return id
}

// ClaimRecipient moves the claim of a placed recipient's message (only Bob's has one) to `age` ago:
// another attempt's claim, live while younger than the send lease.
func (env *TestEnv) ClaimRecipient(ctx context.Context, recipientID int64, age time.Duration) {
	err := env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Update().
		Where(outboundmessage.BroadcastRecipientID(recipientID)).SetClaimedAt(time.Now().Add(-age)).Exec(ctx)
	if err != nil {
		panic(err)
	}
}
