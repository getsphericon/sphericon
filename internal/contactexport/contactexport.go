// Package contactexport is the subject-access Data export of one Contact
// (GDPR Art. 15/20, ADR 0021): a streamed JSON document holding the Contact, its
// Tags, Visitors, all Events, the Unsubscribe/Suppression/Confirmation rows for
// its Destinations and the OutboundMessage / BroadcastRecipient delivery
// metadata. Rendered message bodies are never part of it (no entity read here
// carries one). Each row is projected onto the contract's typed
// ContactExportDocument members by a Mapper, never encoded as the stored entity.
// Large collections are read in keyset pages and written straight to the writer,
// so the document is never buffered whole.
package contactexport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
)

// pageSize bounds how many rows of a large collection are held at once.
const pageSize = 500

// ErrNotFound means no Contact in the Workspace matches the identifier.
var ErrNotFound = errors.New("contactexport: contact not found")

// ErrIdentifier means the request named neither or both of id and email.
var ErrIdentifier = errors.New("contactexport: exactly one of id or email is required")

// Ref is the optional string of a query parameter read as "(value, present)": nil
// when absent. It lets a handler pass its generated Opt parameters straight to Find.
func Ref[T ~string](v T, ok bool) *string {
	if !ok {
		return nil
	}
	return lo.ToPtr(string(v))
}

// Find resolves the Contact to export by exactly one of its id or email, inside
// the Workspace of s: another Workspace's Contact is not found. Both surfaces map its
// two errors (ErrIdentifier: bad request; ErrNotFound) the same way.
func Find(ctx context.Context, s *ent.Scoped, id *int64, email *string) (*ent.Contact, error) {
	if (id == nil) == (email == nil) {
		return nil, ErrIdentifier
	}
	q := s.Contact().Query()
	if id != nil {
		q = q.Where(contact.ID(*id))
	} else {
		q = q.Where(contact.Email(strings.ToLower(strings.TrimSpace(*email))))
	}
	c, err := q.Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return c, err
}

// Stream writes the export of c to w as one JSON object. The Contact and every
// member are read through s, so nothing outside the Workspace can appear.
func Stream(ctx context.Context, s *ent.Scoped, m Mapper, w io.Writer, c *ent.Contact) error {
	visitors, err := s.Visitor().Query().Where(visitor.ContactID(c.ID)).Order(ent.Asc(visitor.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	tags, err := s.Tag().Query().Where(tag.HasContactsWith(contact.ID(c.ID))).Order(ent.Asc(tag.FieldID)).All(ctx)
	if err != nil {
		return err
	}

	// The Contact's Destinations (ADR 0001): its email address. Rows are matched by
	// Destination as well as by contact_id, so a transactional send to the address
	// that never had a Contact is included.
	var destinations []string
	if c.Email != nil {
		destinations = append(destinations, strings.ToLower(*c.Email))
	}
	visitorIDs := lo.Map(visitors, func(v *ent.Visitor, _ int) string { return v.VisitorID })

	unsubscribes, err := s.Unsubscribe().Query().
		Where(unsubscribe.Or(unsubscribe.ContactID(c.ID), unsubscribe.DestinationIn(destinations...))).
		Order(ent.Asc(unsubscribe.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	suppressions, err := s.Suppression().Query().
		Where(suppression.Or(suppression.ContactID(c.ID), suppression.DestinationIn(destinations...))).
		Order(ent.Asc(suppression.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	confirmations, err := s.Confirmation().Query().
		Where(confirmation.Or(confirmation.ContactID(c.ID), confirmation.DestinationIn(destinations...))).
		Order(ent.Asc(confirmation.FieldID)).All(ctx)
	if err != nil {
		return err
	}

	// The keys below are the members of the contract's ContactExportDocument.
	sw := &streamWriter{w: w}
	sw.raw(`{"contact":`)
	sw.value(m.Contact(c))
	sw.raw(`,"tags":`)
	writeList(sw, tags, m.Tag)
	sw.raw(`,"visitors":`)
	writeList(sw, visitors, m.Visitor)
	sw.raw(`,"unsubscribes":`)
	writeList(sw, unsubscribes, m.Unsubscribe)
	sw.raw(`,"suppressions":`)
	writeList(sw, suppressions, m.Suppression)
	sw.raw(`,"confirmations":`)
	writeList(sw, confirmations, m.Confirmation)

	// Events are matched by contact_id and by the Contact's visitor ids (ADR 0021);
	// an empty visitor list matches nothing.
	sw.raw(`,"events":`)
	err = streamPages(ctx, sw, func(after int64) ([]*ent.Event, error) {
		return s.Event().Query().
			Where(event.Or(event.ContactID(c.ID), event.VisitorIDIn(visitorIDs...)), event.IDGT(after)).
			Order(ent.Asc(event.FieldID)).Limit(pageSize).All(ctx)
	}, func(e *ent.Event) int64 { return e.ID }, m.Event)
	if err != nil {
		return err
	}

	sw.raw(`,"outboundMessages":`)
	err = streamPages(ctx, sw, func(after int64) ([]*ent.OutboundMessage, error) {
		return s.OutboundMessage().Query().
			Where(outboundmessage.Or(outboundmessage.ContactID(c.ID), outboundmessage.DestinationIn(destinations...)), outboundmessage.IDGT(after)).
			Order(ent.Asc(outboundmessage.FieldID)).Limit(pageSize).All(ctx)
	}, func(o *ent.OutboundMessage) int64 { return o.ID }, m.OutboundMessage)
	if err != nil {
		return err
	}

	sw.raw(`,"broadcastRecipients":`)
	err = streamPages(ctx, sw, func(after int64) ([]*ent.BroadcastRecipient, error) {
		return s.BroadcastRecipient().Query().
			Where(broadcastrecipient.ContactID(c.ID), broadcastrecipient.IDGT(after)).
			Order(ent.Asc(broadcastrecipient.FieldID)).Limit(pageSize).All(ctx)
	}, func(r *ent.BroadcastRecipient) int64 { return r.ID }, m.BroadcastRecipient)
	if err != nil {
		return err
	}
	sw.raw(`}`)
	return sw.err
}

// Open starts the export of c in the background and returns the stream to hand to
// the HTTP response: the document is produced as the response is read. The
// producer ends with the request's context, so an abandoned download cannot leak it.
func Open(ctx context.Context, s *ent.Scoped, m Mapper, c *ent.Contact) io.Reader {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(Stream(ctx, s, m, pw, c)) }()
	go func() {
		<-ctx.Done()
		pr.CloseWithError(ctx.Err())
	}()
	return pr
}

// Filename is the suggested download name of c's export.
func Filename(c *ent.Contact) string {
	return "contact-" + strconv.FormatInt(c.ID, 10) + "-export.json"
}

// Mapper projects stored rows onto the members of the surface's generated
// ContactExportDocument. Each surface (site, external) generates its own copy of
// those types from the shared TypeSpec model, so a Mapper hands back each member as
// a json.Marshaler and the document is assembled here without knowing which
// surface it serves. Stored rows are never encoded directly: the contract, not
// the ent JSON tags, decides what a person sees.
type Mapper interface {
	Contact(*ent.Contact) json.Marshaler
	Tag(*ent.Tag) json.Marshaler
	Visitor(*ent.Visitor) json.Marshaler
	Event(*ent.Event) json.Marshaler
	Unsubscribe(*ent.Unsubscribe) json.Marshaler
	Suppression(*ent.Suppression) json.Marshaler
	Confirmation(*ent.Confirmation) json.Marshaler
	OutboundMessage(*ent.OutboundMessage) json.Marshaler
	BroadcastRecipient(*ent.BroadcastRecipient) json.Marshaler
}

// streamPages writes a JSON array whose elements are read page by page, ordered
// by ascending id, so at most one page is in memory.
func streamPages[T any](ctx context.Context, sw *streamWriter, page func(after int64) ([]T, error), id func(T) int64, project func(T) json.Marshaler) error {
	sw.raw(`[`)
	var after int64
	first := true
	for sw.err == nil {
		rows, err := page(after)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if !first {
				sw.raw(`,`)
			}
			first = false
			sw.value(project(r))
			after = id(r)
		}
		if len(rows) < pageSize {
			break
		}
	}
	sw.raw(`]`)
	return sw.err
}

// streamWriter remembers the first write or encode error so the document code
// stays linear.
type streamWriter struct {
	w   io.Writer
	err error
}

func (s *streamWriter) raw(text string) {
	if s.err == nil {
		_, s.err = io.WriteString(s.w, text)
	}
}

// writeList writes rows as a JSON array; no rows is [], never null.
func writeList[T any](sw *streamWriter, rows []T, project func(T) json.Marshaler) {
	sw.raw(`[`)
	for i, r := range rows {
		if i > 0 {
			sw.raw(`,`)
		}
		sw.value(project(r))
	}
	sw.raw(`]`)
}

func (s *streamWriter) value(v json.Marshaler) {
	if s.err != nil {
		return
	}
	b, err := v.MarshalJSON()
	if err != nil {
		s.err = err
		return
	}
	_, s.err = s.w.Write(b)
}
