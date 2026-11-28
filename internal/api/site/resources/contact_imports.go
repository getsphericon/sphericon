package resources

import (
	"io"
	"net/url"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
)

// contactImportReportURL is the path the browser follows to download an import's refused rows'
// report: the contract's SiteContactImports report under the /site prefix (ADR 0138).
func contactImportReportURL(l contactimports.ReportLink) string {
	return paths.Site + "/workspaces/" + url.PathEscape(l.Workspace) + "/contact-imports/" + publicid.Encode(l.ImportID) + "/report/" + url.PathEscape(l.Token)
}

// contactImportMapping is the mapping of an import in the contract's shape: each column by its
// kind, and the choices for every row.
func contactImportMapping(c Converter, v contactimports.View) siteapi.ContactImportMapping {
	columns := make([]siteapi.ContactImportColumn, len(v.Mapping.Columns))
	for i, col := range v.Mapping.Columns {
		columns[i] = contactImportColumn(c, col)
	}
	return siteapi.ContactImportMapping{
		Columns:          columns,
		DecimalSeparator: siteapi.ContactImportDecimalSeparator(string(v.Mapping.DecimalSeparator)),
		DefaultCountry:   convert.Nil[siteapi.NilCountryCode](optionalCountry(v.Mapping.DefaultCountry)),
		Tag:              convert.Nil[siteapi.NilName](optionalName(v.Mapping.Tag)),
		OnlyAddNew:       v.Import.OnlyAddNew,
		AsNewSignUps:     v.Import.AsNewSignUps,
	}
}

// optionalCountry is a country code, nil for none.
func optionalCountry(s string) *siteapi.CountryCode {
	if s == "" {
		return nil
	}
	return new(siteapi.CountryCode(s))
}

// optionalName is a name, nil for none.
func optionalName(s string) *siteapi.Name {
	if s == "" {
		return nil
	}
	return new(siteapi.Name(s))
}

// contactImportColumn is a column of a mapping by its kind. A stored mapping is checked when it is
// saved, so a kind it does not know is an ignored column's never.
func contactImportColumn(c Converter, col csvimport.Column) siteapi.ContactImportColumn {
	format := siteapi.NilContactImportDateFormat{Null: true}
	if col.DateFormat != "" {
		format = siteapi.NewNilContactImportDateFormat(c.ContactImportDateFormat(col.DateFormat))
	}
	switch col.Kind {
	case csvimport.KindKey:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportKeyColumnContactImportColumnSum(siteapi.ContactImportKeyColumn{
			Kind: siteapi.ContactImportKeyColumnKindKey, Key: siteapi.ContactImportKey(col.Key),
		})}
	case csvimport.KindAttribute:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportAttributeColumnContactImportColumnSum(siteapi.ContactImportAttributeColumn{
			Kind: siteapi.ContactImportAttributeColumnKindAttribute, Attribute: siteapi.ContactImportAttribute(col.Attribute), DateFormat: format,
		})}
	case csvimport.KindCustomField:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportCustomFieldColumnContactImportColumnSum(siteapi.ContactImportCustomFieldColumn{
			Kind: siteapi.ContactImportCustomFieldColumnKindCustomField, Key: col.Key, DateFormat: format,
		})}
	case csvimport.KindNewCustomField:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportNewCustomFieldColumnContactImportColumnSum(siteapi.ContactImportNewCustomFieldColumn{
			Kind: siteapi.ContactImportNewCustomFieldColumnKindNewCustomField, Key: col.Key, Label: siteapi.Name(col.Label),
			Type: siteapi.CustomFieldType(col.FieldType), DateFormat: format,
		})}
	case csvimport.KindTags:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportTagsColumnContactImportColumnSum(siteapi.ContactImportTagsColumn{
			Kind: siteapi.ContactImportTagsColumnKindTags,
		})}
	case csvimport.KindRefusal, csvimport.KindConsent, csvimport.KindEngagementTracking, csvimport.KindTime:
		return contactImportFactColumn(c, col, format)
	case csvimport.KindIgnore:
	}
	return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportIgnoreColumnContactImportColumnSum(siteapi.ContactImportIgnoreColumn{
		Kind: siteapi.ContactImportIgnoreColumnKindIgnore,
	})}
}

// contactImportRefusal is why a previewed row would be refused, null when it would be written.
func contactImportRefusal(c Converter, refusal *csvimport.Issue) siteapi.NilContactImportIssue {
	if refusal == nil {
		return siteapi.NilContactImportIssue{Null: true}
	}
	return siteapi.NewNilContactImportIssue(c.ContactImportIssueToResource(*refusal))
}

// fileIn is an uploaded file's body, read by the module as it is.
func fileIn(r io.Reader) io.Reader { return r }

// contactImportMappingIn is the mapping a request saves: each column by its kind, a date format
// through the enum's method, and the choices for every row ("" for a default country or a Tag the
// request leaves null).
func contactImportMappingIn(c Inputs, source *siteapi.ContactImportMappingInput) csvimport.Mapping {
	columns := make([]csvimport.Column, len(source.Columns))
	for i := range source.Columns {
		columns[i] = contactImportColumnIn(c, &source.Columns[i])
	}
	return csvimport.Mapping{
		Columns:          columns,
		DecimalSeparator: csvimport.DecimalSeparator(string(source.DecimalSeparator)),
		DefaultCountry:   lo.FromPtr(convert.StringPtr(source.DefaultCountry)),
		Tag:              lo.FromPtr(convert.StringPtr(source.Tag)),
	}
}

// contactImportColumnIn is a column a request maps, by its kind; a column of no kind writes
// nothing, which the save refuses.
func contactImportColumnIn(c Inputs, col *siteapi.ContactImportColumnInput) csvimport.Column {
	dateFormat := func(f siteapi.NilContactImportDateFormat) csvimport.DateFormat {
		if v, ok := f.Get(); ok {
			return c.ContactImportDateFormatIn(v)
		}
		return ""
	}
	switch col.OneOf.Type {
	case siteapi.ContactImportKeyColumnInputContactImportColumnInputSum:
		return csvimport.Column{Kind: csvimport.KindKey, Key: string(col.OneOf.ContactImportKeyColumnInput.Key)}
	case siteapi.ContactImportAttributeColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportAttributeColumnInput
		return csvimport.Column{Kind: csvimport.KindAttribute, Attribute: string(v.Attribute), DateFormat: dateFormat(v.DateFormat)}
	case siteapi.ContactImportCustomFieldColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportCustomFieldColumnInput
		return csvimport.Column{Kind: csvimport.KindCustomField, Key: v.Key, DateFormat: dateFormat(v.DateFormat)}
	case siteapi.ContactImportNewCustomFieldColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportNewCustomFieldColumnInput
		return csvimport.Column{
			Kind: csvimport.KindNewCustomField, Key: v.Key, Label: string(v.Label),
			FieldType: string(v.Type), DateFormat: dateFormat(v.DateFormat),
		}
	case siteapi.ContactImportTagsColumnInputContactImportColumnInputSum:
		return csvimport.Column{Kind: csvimport.KindTags}
	case siteapi.ContactImportIgnoreColumnInputContactImportColumnInputSum:
		return csvimport.Column{Kind: csvimport.KindIgnore}
	case siteapi.ContactImportRefusalColumnInputContactImportColumnInputSum,
		siteapi.ContactImportConsentColumnInputContactImportColumnInputSum,
		siteapi.ContactImportEngagementTrackingColumnInputContactImportColumnInputSum,
		siteapi.ContactImportTimeColumnInputContactImportColumnInputSum:
		return contactImportFactColumnIn(c, col)
	}
	return csvimport.Column{}
}

// contactImportFactColumn is a refusal, consent, Engagement tracking or time column of a mapping.
func contactImportFactColumn(c Converter, col csvimport.Column, format siteapi.NilContactImportDateFormat) siteapi.ContactImportColumn {
	fact := lo.FromPtr(col.Fact)
	switch col.Kind {
	case csvimport.KindRefusal:
		values := make([]siteapi.ContactImportRefusalValue, len(fact.Refusals))
		for i, v := range fact.Refusals {
			values[i] = siteapi.ContactImportRefusalValue{Value: v.Value, Refusal: c.ContactImportRefusal(v.Refusal)}
		}
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportRefusalColumnContactImportColumnSum(siteapi.ContactImportRefusalColumn{
			Kind: siteapi.ContactImportRefusalColumnKindRefusal, Channel: c.ContactImportRefusalChannel(fact.Channel), Values: values,
		})}
	case csvimport.KindConsent:
		ev := lo.FromPtr(fact.Evidence)
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportConsentColumnContactImportColumnSum(siteapi.ContactImportConsentColumn{
			Kind: siteapi.ContactImportConsentColumnKindConsent, Channel: c.ContactImportConsentChannel(fact.Channel),
			Values: lo.CoalesceSliceOrEmpty(fact.Values),
			Evidence: siteapi.ContactImportConsentEvidence{
				Wording: evidenceText(ev.Wording), WordingColumn: evidenceColumn(ev.Wording),
				Source: evidenceText(ev.Source), SourceColumn: evidenceColumn(ev.Source),
				Brand: evidenceText(ev.Brand), BrandColumn: evidenceColumn(ev.Brand),
			},
		})}
	case csvimport.KindEngagementTracking:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportEngagementTrackingColumnContactImportColumnSum(siteapi.ContactImportEngagementTrackingColumn{
			Kind: siteapi.ContactImportEngagementTrackingColumnKindEngagementTracking, Channel: c.ContactImportConsentChannel(fact.Channel),
			Granted: lo.CoalesceSliceOrEmpty(fact.Values), Denied: lo.CoalesceSliceOrEmpty(fact.Denied),
		})}
	case csvimport.KindTime:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportTimeColumnContactImportColumnSum(siteapi.ContactImportTimeColumn{
			Kind: siteapi.ContactImportTimeColumnKindTime, Of: nilColumn(fact.Of), DateFormat: format,
		})}
	case csvimport.KindKey, csvimport.KindAttribute, csvimport.KindCustomField, csvimport.KindNewCustomField,
		csvimport.KindTags, csvimport.KindIgnore:
		// No fact column: contactImportColumn maps it.
	}
	return siteapi.ContactImportColumn{}
}

// evidenceText is the text of an evidence part, null when a column holds it or none is given yet.
func evidenceText(p csvimport.EvidencePart) siteapi.NilString {
	if p.Column != nil || p.Text == "" {
		return siteapi.NilString{Null: true}
	}
	return siteapi.NewNilString(p.Text)
}

// evidenceColumn is the column of an evidence part, null when its text is given.
func evidenceColumn(p csvimport.EvidencePart) siteapi.NilInt32 { return nilColumn(p.Column) }

// nilColumn is a column of the file, counted from 0, null for none.
func nilColumn(at *int) siteapi.NilInt32 {
	if at == nil {
		return siteapi.NilInt32{Null: true}
	}
	return siteapi.NewNilInt32(count(*at))
}

// contactImportFactColumnIn is a refusal, consent, Engagement tracking or time column a request
// maps.
func contactImportFactColumnIn(c Inputs, col *siteapi.ContactImportColumnInput) csvimport.Column {
	switch col.OneOf.Type {
	case siteapi.ContactImportRefusalColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportRefusalColumnInput
		values := make([]csvimport.RefusalValue, len(v.Values))
		for i, value := range v.Values {
			values[i] = csvimport.RefusalValue{Value: value.Value, Refusal: c.ContactImportRefusalIn(value.Refusal)}
		}
		return csvimport.Column{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{Channel: c.ContactImportRefusalChannelIn(v.Channel), Refusals: values}}
	case siteapi.ContactImportConsentColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportConsentColumnInput
		ev := v.Evidence
		return csvimport.Column{Kind: csvimport.KindConsent, Fact: &csvimport.Fact{Channel: c.ContactImportConsentChannelIn(v.Channel), Values: v.Values, Evidence: &csvimport.Evidence{
			Wording: evidencePartIn(ev.Wording, ev.WordingColumn),
			Source:  evidencePartIn(ev.Source, ev.SourceColumn),
			Brand:   evidencePartIn(ev.Brand, ev.BrandColumn),
		}}}
	case siteapi.ContactImportEngagementTrackingColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportEngagementTrackingColumnInput
		return csvimport.Column{Kind: csvimport.KindEngagementTracking, Fact: &csvimport.Fact{Channel: c.ContactImportConsentChannelIn(v.Channel), Values: v.Granted, Denied: v.Denied}}
	case siteapi.ContactImportTimeColumnInputContactImportColumnInputSum:
		v := col.OneOf.ContactImportTimeColumnInput
		var format csvimport.DateFormat
		if f, ok := v.DateFormat.Get(); ok {
			format = c.ContactImportDateFormatIn(f)
		}
		fact := &csvimport.Fact{}
		if of, ok := v.Of.Get(); ok {
			fact.Of = new(int(of))
		}
		return csvimport.Column{Kind: csvimport.KindTime, DateFormat: format, Fact: fact}
	case siteapi.ContactImportKeyColumnInputContactImportColumnInputSum,
		siteapi.ContactImportAttributeColumnInputContactImportColumnInputSum,
		siteapi.ContactImportCustomFieldColumnInputContactImportColumnInputSum,
		siteapi.ContactImportNewCustomFieldColumnInputContactImportColumnInputSum,
		siteapi.ContactImportTagsColumnInputContactImportColumnInputSum,
		siteapi.ContactImportIgnoreColumnInputContactImportColumnInputSum:
		// No fact column: contactImportColumnIn maps it.
	}
	return csvimport.Column{}
}

// evidencePartIn is an evidence part a request gives: its text, or the column that holds it.
func evidencePartIn(text siteapi.NilString, column siteapi.NilInt32) csvimport.EvidencePart {
	var p csvimport.EvidencePart
	if v, ok := text.Get(); ok {
		p.Text = v
	}
	if v, ok := column.Get(); ok {
		p.Column = new(int(v))
	}
	return p
}
