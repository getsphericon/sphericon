package contactimports

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Refusals of a mapping. Each names the column it refuses, by its index under `columns`.
var (
	// ErrColumnCount means the mapping has another number of columns than the file.
	ErrColumnCount = validation.RefuseField(fieldname.Columns, "contactimports: the mapping has another number of columns than the file", i18n.ErrorsContactImportColumnCount)
	// ErrUnknownTarget means a column names a key, an attribute or a kind no import writes.
	ErrUnknownTarget = validation.RefuseField(fieldname.Columns, "contactimports: a column names nothing an import writes", i18n.ErrorsContactImportUnknownTarget)
	// ErrUnknownCustomField means a column names a Custom field the Workspace has not.
	ErrUnknownCustomField = validation.RefuseField(fieldname.Columns, "contactimports: a column names a custom field the workspace has not", i18n.ErrorsContactImportUnknownCustomField)
	// ErrDateFormatRequired means a date column (the birthday, a datetime Custom field) names no
	// format: the Member chooses one, which is never guessed.
	ErrDateFormatRequired = validation.RefuseField(fieldname.Columns, "contactimports: a date column needs its format", i18n.ErrorsContactImportDateFormatRequired)
	// ErrTargetRepeated means two columns write the same key, attribute or Custom field.
	ErrTargetRepeated = validation.RefuseField(fieldname.Columns, "contactimports: two columns write the same value", i18n.ErrorsContactImportTargetRepeated)
	// ErrDecimalSeparator means a decimal separator other than a point or a comma.
	ErrDecimalSeparator = validation.RefuseField(fieldname.DecimalSeparator, "contactimports: the decimal separator is a point or a comma", i18n.ErrorsContactImportDecimalSeparator)
	// ErrDefaultCountry means a default country that is no two-letter country code.
	ErrDefaultCountry = validation.RefuseField(fieldname.DefaultCountry, "contactimports: the default country is no country code", i18n.ErrorsContactImportDefaultCountry)
)

// keys and attributes are what a key and an attribute column may name, by the contract's names.
var (
	keys       = []string{fieldname.SubjectID, fieldname.Email, fieldname.Phone}
	attributes = []string{
		fieldname.FirstName, fieldname.LastName, fieldname.TimeZone, fieldname.Country,
		fieldname.Locale, fieldname.City, fieldname.Avatar, fieldname.Birthday,
	}
)

// MappingInput is a mapping a Member saves, the "only add new Contacts" choice and the "treat
// them as new sign-ups" one, which makes the run's Events live (events.ImportAsNew). Role is the
// Role of the Member who saves it, whose Permissions a column that creates a Custom field asks.
type MappingInput struct {
	Mapping      csvimport.Mapping
	OnlyAddNew   bool
	AsNewSignUps bool
	// Role is the saving Member's: a new Custom field needs custom_fields:manage, a Suppression
	// suppressions:manage (ADR 0138).
	Role membership.Role
}

// SaveMapping saves how the import id reads its file and makes it `ready`. A column per column of
// the file, each naming what it writes: a key or an attribute by the contract's name, a Custom field
// the Workspace has by its key, a new Custom field (checkedNewCustomField, which needs
// custom_fields:manage of the Member's Role), Tags, or nothing; no two writing the same value; a date column (the
// birthday, a datetime Custom field) with its format and no other column with one. The decimal
// separator is a point or a comma, the default country a country code or none, the Tag for every
// row a Tag's name or none. An import that runs or ran keeps its mapping (lifecycle's
// ErrInvalidTransition).
func (m *Module) SaveMapping(ctx context.Context, s *ent.Scoped, id int64, in MappingInput) (View, error) {
	imp, err := load(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	if err := mayCreateFields(in.Role, in.Mapping); err != nil {
		return View{}, err
	}
	mapping, err := checkedMapping(ctx, s, imp, in.Mapping)
	if err != nil {
		return View{}, err
	}
	if err := mayRecord(mapping, in.Role); err != nil {
		return View{}, err
	}
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return m.machine(ts, imp.ID, func(q *ent.ContactImportScopedUpdate) {
			q.SetMapping(mapping).SetOnlyAddNew(in.OnlyAddNew).SetAsNewSignUps(in.AsNewSignUps)
		}).FireCtx(ctx, triggerSaveMapping)
	})
	if err != nil {
		return View{}, err
	}
	return m.Get(ctx, s, id)
}

// checkedMapping is mapping as SaveMapping keeps it, or its refusal.
func checkedMapping(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport, mapping csvimport.Mapping) (csvimport.Mapping, error) {
	if err := validation.Items(fieldname.Columns, len(mapping.Columns), 0, bounds.ContactImportColumnsMaxItems); err != nil {
		return csvimport.Mapping{}, err
	}
	if len(mapping.Columns) != len(imp.Header) {
		return csvimport.Mapping{}, ErrColumnCount
	}
	if mapping.DecimalSeparator != csvimport.DecimalPoint && mapping.DecimalSeparator != csvimport.DecimalComma {
		return csvimport.Mapping{}, ErrDecimalSeparator
	}
	if mapping.DefaultCountry != "" {
		country, reason := contacts.ReadCountry(mapping.DefaultCountry)
		if reason != "" {
			return csvimport.Mapping{}, ErrDefaultCountry
		}
		mapping.DefaultCountry = country
	}
	if mapping.Tag != "" {
		tag, err := validation.Name(mapping.Tag)
		if err != nil {
			return csvimport.Mapping{}, refusalAt(err, fieldname.Tag)
		}
		mapping.Tag = tag
	}
	fields, err := customFieldTypes(ctx, s, mapping.Columns)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	written := map[string]bool{}
	out := mapping
	out.Columns = make([]csvimport.Column, len(mapping.Columns))
	for i, col := range mapping.Columns {
		at := validation.Nested(fieldname.Columns, strconv.Itoa(i))
		checked, target, err := checkedColumn(col, fields)
		if err != nil {
			return csvimport.Mapping{}, refusalAt(err, at)
		}
		if target != "" && written[target] {
			return csvimport.Mapping{}, ErrTargetRepeated.On(at)
		}
		written[target] = target != ""
		out.Columns[i] = checked
	}
	if err := pairs(out.Columns); err != nil {
		return csvimport.Mapping{}, err
	}
	return out, nil
}

// refusalAt is the refusal of a column named at its place under `columns`.
func refusalAt(err error, at string) error {
	if r, ok := errors.AsType[*validation.RefusalError](err); ok {
		return r.On(at)
	}
	return err
}

// checkedColumn is a column as a mapping keeps it and the value it writes (its kind and name, ""
// for Tags and an ignored column), or its refusal. A date format is kept on a date column only.
func checkedColumn(col csvimport.Column, fields map[string]customfield.Type) (csvimport.Column, string, error) {
	switch col.Kind {
	case csvimport.KindKey:
		if !lo.Contains(keys, col.Key) {
			return col, "", ErrUnknownTarget
		}
		return csvimport.Column{Kind: col.Kind, Key: col.Key}, "key:" + col.Key, nil
	case csvimport.KindAttribute:
		if !lo.Contains(attributes, col.Attribute) {
			return col, "", ErrUnknownTarget
		}
		out := csvimport.Column{Kind: col.Kind, Attribute: col.Attribute}
		if col.Attribute == fieldname.Birthday {
			if col.DateFormat.Layout() == "" {
				return col, "", ErrDateFormatRequired
			}
			out.DateFormat = col.DateFormat
		}
		return out, "attribute:" + col.Attribute, nil
	case csvimport.KindCustomField:
		t, ok := fields[col.Key]
		if !ok {
			return col, "", ErrUnknownCustomField.Args(map[string]any{"Key": col.Key})
		}
		out := csvimport.Column{Kind: col.Kind, Key: col.Key}
		if t == customfield.TypeDatetime {
			if col.DateFormat.Layout() == "" {
				return col, "", ErrDateFormatRequired
			}
			out.DateFormat = col.DateFormat
		}
		return out, "customField:" + col.Key, nil
	case csvimport.KindNewCustomField:
		return checkedNewCustomField(col, fields)
	case csvimport.KindTags, csvimport.KindIgnore:
		return csvimport.Column{Kind: col.Kind}, "", nil
	case csvimport.KindRefusal, csvimport.KindTime:
		out, err := checkedFactColumn(&col)
		return out, "", err
	case csvimport.KindConsent, csvimport.KindEngagementTracking:
		out, err := checkedFactColumn(&col)
		return out, string(col.Kind) + ":" + lo.FromPtr(col.Fact).Channel, err
	}
	return col, "", ErrUnknownTarget
}

// customFieldTypes are the types of the Workspace's Custom fields the columns name, by key.
func customFieldTypes(ctx context.Context, s *ent.Scoped, columns []csvimport.Column) (map[string]customfield.Type, error) {
	named := lo.FilterMap(columns, func(c csvimport.Column, _ int) (string, bool) {
		if c.Kind == csvimport.KindNewCustomField {
			return contacts.NewCustomFieldKey(c.Key), true
		}
		return c.Key, c.Kind == csvimport.KindCustomField
	})
	defined, err := s.CustomField().Query().Where(customfield.KeyIn(named...)).All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.SliceToMap(defined, func(f *ent.CustomField) (string, customfield.Type) { return f.Key, f.Type }), nil
}

// sampleRows are the rows a proposal and a preview read: the first ones of the file.
const sampleRows = 20

// propose is the mapping proposed for imp's file before one is saved (ADR 0138): a header that
// names an alias key, a core attribute or a Custom field of the Workspace (in any case, with or
// without spaces, dashes or underscores), or Tags, is mapped to it, each target once, the first
// header that names it; a header of a CSV export of Contacts is mapped as the export means it
// (exported), so a Workspace's export imports into another as it is, but for a column the import's
// Member's Role may not save (a new Custom field, a Suppression), which is ignored; a blank or repeated header and
// any other is ignored. A date column gets the one format its sample's cells all read in (read
// back as written, csvfile.Unguarded), none when several do or none does: the Member chooses then;
// a column whose sample is blank gets the format the export writes (ISO for a date, RFC 3339 for a
// time). Numbers are proposed with a point, a comma in a file separated by semicolons; the Tag for
// every row is "Imported" and the date.
func (m *Module) propose(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) (csvimport.Mapping, error) {
	sample, err := sampleOf(ctx, s, imp)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	defined, err := s.CustomField().Query().All(ctx)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	zone, err := localcalendar.Zone(nil, ws.Timezone)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	targets := map[string]csvimport.Column{"tags": {Kind: csvimport.KindTags}}
	for _, k := range keys {
		targets[headerKey(k)] = csvimport.Column{Kind: csvimport.KindKey, Key: k}
	}
	for _, a := range attributes {
		targets[headerKey(a)] = csvimport.Column{Kind: csvimport.KindAttribute, Attribute: a}
	}
	fields := lo.SliceToMap(defined, func(f *ent.CustomField) (string, customfield.Type) { return f.Key, f.Type })
	for _, f := range defined {
		if _, taken := targets[headerKey(f.Key)]; !taken {
			targets[headerKey(f.Key)] = csvimport.Column{Kind: csvimport.KindCustomField, Key: f.Key}
		}
	}
	mapping := csvimport.Mapping{
		DecimalSeparator: lo.Ternary(imp.Delimiter == contactimport.DelimiterSemicolon, csvimport.DecimalComma, csvimport.DecimalPoint),
		Tag:              "Imported " + m.now().In(zone).Format("2006-01-02"),
	}
	ms, err := m.membership(ctx, s, imp)
	if err != nil {
		return csvimport.Mapping{}, err
	}
	export := exported{fields: fields, sep: mapping.DecimalSeparator}
	if ms != nil {
		export.role = ms.Role
	}
	used := map[string]bool{}
	for at, h := range imp.Header {
		cells := lo.Map(sample, func(cells []string, _ int) string { return csvfile.Unguarded(cellAt(cells, at)) })
		col, ok := targets[headerKey(h)]
		if !ok {
			col, ok = export.column(h, at, cells)
		}
		target := lo.CoalesceOrEmpty(targetOf(col), "header:"+headerKey(h))
		if !ok || headerKey(h) == "" || used[target] {
			mapping.Columns = append(mapping.Columns, csvimport.Column{Kind: csvimport.KindIgnore})
			continue
		}
		used[target] = true
		switch {
		case col.Attribute == fieldname.Birthday:
			col.DateFormat = proposedFormat(cells, csvimport.DateISO)
		case col.Kind == csvimport.KindCustomField && fields[col.Key] == customfield.TypeDatetime && col.DateFormat == "":
			col.DateFormat = proposedFormat(cells, csvimport.DateTimeRFC3339)
		}
		mapping.Columns = append(mapping.Columns, col)
	}
	times(mapping.Columns, imp.Header)
	return mapping, nil
}

// targetOf is the value a proposed column writes, which no other proposed column may write: its
// key, attribute, Custom field, Tags or consent; "" for a refusal or a time column, which only a
// repeated header repeats.
func targetOf(col csvimport.Column) string {
	switch col.Kind {
	case csvimport.KindKey:
		return "key:" + col.Key
	case csvimport.KindAttribute:
		return "attribute:" + col.Attribute
	case csvimport.KindCustomField, csvimport.KindNewCustomField:
		return "customField:" + col.Key
	case csvimport.KindTags:
		return "tags"
	case csvimport.KindConsent:
		return "consent:" + col.Fact.Channel
	case csvimport.KindRefusal, csvimport.KindTime, csvimport.KindEngagementTracking, csvimport.KindIgnore:
	}
	return ""
}

// headerKey is a header as a proposal matches it: lower case, without spaces, dashes and
// underscores.
func headerKey(h string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(h)))
}

// proposedFormat is the one date format every non-blank cell reads in, "" when none or several do,
// blank when every cell is blank.
func proposedFormat(cells []string, blank csvimport.DateFormat) csvimport.DateFormat {
	filled := lo.Filter(cells, func(c string, _ int) bool { return strings.TrimSpace(c) != "" })
	if len(filled) == 0 {
		return blank
	}
	fits := lo.Filter(csvimport.DateFormats, func(f csvimport.DateFormat, _ int) bool {
		return lo.EveryBy(filled, func(c string) bool { _, reason := contacts.ReadDatetime(c, f); return reason == "" })
	})
	// Dates alone read alike in ISO and in RFC 3339 (its full-date): ISO is proposed then.
	if lo.Contains(fits, csvimport.DateISO) {
		fits = lo.Without(fits, csvimport.DateTimeRFC3339)
	}
	if len(fits) != 1 {
		return ""
	}
	return fits[0]
}

// cellAt is the cell of a row at a column, "" past its end.
func cellAt(cells []string, at int) string {
	if at < len(cells) {
		return cells[at]
	}
	return ""
}

// sampleOf are the cells of the first rows of imp's file.
func sampleOf(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) ([][]string, error) {
	var sample [][]string
	for rec, err := range records(ctx, s, imp) {
		if err != nil {
			return nil, err
		}
		sample = append(sample, rec.cells)
		if len(sample) == sampleRows {
			break
		}
	}
	return sample, nil
}
