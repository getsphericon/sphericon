package brandkit

import (
	"encoding/json"
	"fmt"

	"github.com/getsphericon/sphericon/internal/i18n"
)

// Starter is one starter Template as it is created: an ordinary Template's name, subject and
// editor document, with the Brand kit's values already written into the document.
type Starter struct {
	Name    string
	Subject string
	Doc     json.RawMessage
}

// node is one node of the editor's JSON document.
type node = map[string]any

// Starters builds the four starter Templates (a welcome, a receipt, a digest and a one-button
// message) in the Brand kit's brand. The values go into the documents' `style` attributes, so
// the stored body is the brand as it was when the Template was made (ADR 0040): the kit is not
// read again, and editing it later never changes these. Names and copy are in the instance
// locale.
func Starters(k Kit) []Starter {
	b := brand{k}
	return []Starter{
		b.starter("welcome",
			b.logo(),
			b.heading(1, i18n.T("brandkit.welcome.heading", mergeTags)),
			b.paragraph(i18n.T("brandkit.welcome.body", nil)),
			b.button(i18n.T("brandkit.welcome.button", nil)),
		),
		b.starter("receipt",
			b.logo(),
			b.heading(1, i18n.T("brandkit.receipt.heading", nil)),
			b.paragraph(i18n.T("brandkit.receipt.body", nil)),
			b.paragraph(i18n.T("brandkit.receipt.order", mergeTags)),
			b.paragraph(i18n.T("brandkit.receipt.total", mergeTags)),
		),
		b.starter("digest",
			b.logo(),
			b.heading(1, i18n.T("brandkit.digest.heading", nil)),
			b.paragraph(i18n.T("brandkit.digest.intro", nil)),
			b.heading(2, i18n.T("brandkit.digest.first_title", nil)),
			b.paragraph(i18n.T("brandkit.digest.first_body", nil)),
			b.rule(),
			b.heading(2, i18n.T("brandkit.digest.second_title", nil)),
			b.paragraph(i18n.T("brandkit.digest.second_body", nil)),
		),
		b.starter("button",
			b.logo(),
			b.paragraph(i18n.T("brandkit.button.body", nil)),
			b.button(i18n.T("brandkit.button.button", nil)),
		),
	}
}

// mergeTags are the Liquid tags the copy carries, passed as data so the catalogs hold no
// template syntax of their own.
var mergeTags = map[string]any{
	"FirstName":   "{{ first_name }}",
	"OrderNumber": "{{ order_number }}",
	"Total":       "{{ total }}",
}

// brand turns the kit into document nodes.
type brand struct{ kit Kit }

func (b brand) starter(key string, nodes ...node) Starter {
	content := make([]node, 0, len(nodes)+2)
	for _, n := range nodes {
		if n != nil {
			content = append(content, n)
		}
	}
	if b.kit.FooterText != "" {
		content = append(content, b.rule(), b.footer())
	}
	doc := node{"type": "doc", "content": []node{{
		"type":    "section",
		"attrs":   node{"style": "background-color:" + b.kit.BackgroundColor},
		"content": content,
	}}}
	raw, err := json.Marshal(doc)
	if err != nil {
		// A map of strings and numbers always marshals.
		panic(fmt.Sprintf("brandkit: marshal the %s starter: %v", key, err))
	}
	return Starter{
		Name:    i18n.T("brandkit."+key+".name", nil),
		Subject: i18n.T("brandkit."+key+".subject", nil),
		Doc:     raw,
	}
}

func text(s string) node {
	return node{"type": "text", "text": s}
}

func (b brand) heading(level int, s string) node {
	return node{
		"type": "heading",
		"attrs": node{
			"level": level,
			"style": fmt.Sprintf("color:%s;font-family:%s", b.kit.TextColor, b.kit.HeadingFont),
		},
		"content": []node{text(s)},
	}
}

func (b brand) paragraph(s string) node {
	return node{
		"type": "paragraph",
		"attrs": node{
			"style": fmt.Sprintf("color:%s;font-family:%s", b.kit.TextColor, b.kit.BodyFont),
		},
		"content": []node{text(s)},
	}
}

func (b brand) footer() node {
	return node{
		"type": "paragraph",
		"attrs": node{
			"style": fmt.Sprintf("color:%s;font-family:%s;font-size:12px", b.kit.TextColor, b.kit.BodyFont),
		},
		"content": []node{text(b.kit.FooterText)},
	}
}

func (b brand) button(label string) node {
	return node{
		"type": "button",
		"attrs": node{
			"href":      "https://example.com",
			"alignment": "left",
			"style": fmt.Sprintf("background-color:%s;color:%s;border-radius:%dpx;font-family:%s",
				b.kit.AccentColor, b.kit.ButtonTextColor, b.kit.ButtonRadius, b.kit.BodyFont),
		},
		"content": []node{text(label)},
	}
}

func (b brand) rule() node {
	return node{"type": "horizontalRule"}
}

// logo is the kit's logo as an image, or nil when it has none.
func (b brand) logo() node {
	if b.kit.LogoURL == "" {
		return nil
	}
	return node{
		"type":  "image",
		"attrs": node{"src": b.kit.LogoURL, "alt": i18n.T("brandkit.logo_alt", nil), "alignment": "left", "width": "120", "height": "auto"},
	}
}
