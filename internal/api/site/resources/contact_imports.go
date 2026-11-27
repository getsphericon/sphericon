package resources

import (
	"io"
	"time"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
)

// DownloadLinkView is a download link as the answer shows it: the path the browser follows,
// which the handler builds from the link's token, and when the link expires.
type DownloadLinkView struct {
	URL       string
	ExpiresAt time.Time
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
		DecimalSeparator: siteapi.ContactImportDecimalSeparator(v.Mapping.DecimalSeparator),
		DefaultCountry:   convert.Nil[siteapi.NilCountryCode](optionalCountry(v.Mapping.DefaultCountry)),
		Tag:              convert.Nil[siteapi.NilName](optionalName(v.Mapping.Tag)),
		OnlyAddNew:       v.Import.OnlyAddNew,
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
	case csvimport.KindTags:
		return siteapi.ContactImportColumn{OneOf: siteapi.NewContactImportTagsColumnContactImportColumnSum(siteapi.ContactImportTagsColumn{
			Kind: siteapi.ContactImportTagsColumnKindTags,
		})}
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
		DecimalSeparator: string(source.DecimalSeparator),
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
	case siteapi.ContactImportTagsColumnInputContactImportColumnInputSum:
		return csvimport.Column{Kind: csvimport.KindTags}
	case siteapi.ContactImportIgnoreColumnInputContactImportColumnInputSum:
		return csvimport.Column{Kind: csvimport.KindIgnore}
	}
	return csvimport.Column{}
}
