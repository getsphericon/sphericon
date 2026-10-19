package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BrandKit is a Workspace's visual identity as data (GLOSSARY "Brand kit", ADR 0040): at most one
// row per Workspace, created by its first save. Its values are written into a message when the
// message is authored and never resolved at send time, so no message holds a reference to it.
type BrandKit struct {
	ent.Schema
}

func (BrandKit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "brand_kits"},
		Audited{Action: "brand_kit"},
	}
}

func (BrandKit) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "brand_kits"}}
}

func (BrandKit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// Colors are #rrggbb; the module checks them on write.
		field.String("accent_color"),
		field.String("text_color"),
		field.String("background_color"),
		// CSS font-family lists of web-safe families.
		field.String("heading_font"),
		field.String("body_font"),
		// Absolute http(s) URL of the logo image; empty for none.
		field.String("logo_url").
			Default(""),
		field.String("button_text_color"),
		// The button's corner radius in pixels.
		field.Int("button_radius"),
		// Plain text printed under the starter messages; empty for none.
		field.String("footer_text").
			Default(""),
	}
}

func (BrandKit) Indexes() []ent.Index {
	return []ent.Index{
		// One Brand kit per Workspace.
		index.Fields("workspace_id").Unique(),
	}
}
