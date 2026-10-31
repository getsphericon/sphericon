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
		b.starter(i18n.BrandkitWelcomeName, i18n.BrandkitWelcomeSubject,
			b.logo(),
			b.heading(1, i18n.T(i18n.BrandkitWelcomeHeading, mergeTags)),
			b.paragraph(i18n.T(i18n.BrandkitWelcomeBody, nil)),
			b.button(i18n.T(i18n.BrandkitWelcomeButton, nil)),
		),
		b.starter(i18n.BrandkitReceiptName, i18n.BrandkitReceiptSubject,
			b.logo(),
			b.heading(1, i18n.T(i18n.BrandkitReceiptHeading, nil)),
			b.paragraph(i18n.T(i18n.BrandkitReceiptBody, nil)),
			b.paragraph(i18n.T(i18n.BrandkitReceiptOrder, mergeTags)),
			b.paragraph(i18n.T(i18n.BrandkitReceiptTotal, mergeTags)),
		),
		b.starter(i18n.BrandkitDigestName, i18n.BrandkitDigestSubject,
			b.logo(),
			b.heading(1, i18n.T(i18n.BrandkitDigestHeading, nil)),
			b.paragraph(i18n.T(i18n.BrandkitDigestIntro, nil)),
			b.heading(2, i18n.T(i18n.BrandkitDigestFirstTitle, nil)),
			b.paragraph(i18n.T(i18n.BrandkitDigestFirstBody, nil)),
			b.rule(),
			b.heading(2, i18n.T(i18n.BrandkitDigestSecondTitle, nil)),
			b.paragraph(i18n.T(i18n.BrandkitDigestSecondBody, nil)),
		),
		b.starter(i18n.BrandkitButtonName, i18n.BrandkitButtonSubject,
			b.logo(),
			b.paragraph(i18n.T(i18n.BrandkitButtonBody, nil)),
			b.button(i18n.T(i18n.BrandkitButtonButton, nil)),
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

func (b brand) starter(name, subject i18n.ID, nodes ...node) Starter {
	content := make([]node, 0, len(nodes)+2)
	for _, n := range nodes {
		if n != nil {
			content = append(content, n)
		}
	}
	if b.kit.FooterText != "" {
		content = append(content, b.rule(), b.footer())
	}
	section := element("section", styled("background-color:"+b.kit.BackgroundColor), content...)
	doc := element("doc", nil, section)
	raw, err := json.Marshal(doc)
	if err != nil {
		// A map of strings and numbers always marshals.
		panic(fmt.Sprintf("brandkit: marshal the %s starter: %v", name.Key(), err))
	}
	return Starter{
		Name:    i18n.T(name, nil),
		Subject: i18n.T(subject, nil),
		Doc:     raw,
	}
}

// element is a node of the given type; attrs and content are left out when empty.
func element(typ string, attrs node, content ...node) node {
	n := node{"type": typ}
	if len(attrs) > 0 {
		n["attrs"] = attrs
	}
	if len(content) > 0 {
		n["content"] = content
	}
	return n
}

// styled is the attributes of a node that carries only an inline style.
func styled(style string) node {
	return node{"style": style}
}

func text(s string) node {
	return node{"type": "text", "text": s}
}

func (b brand) heading(level int, s string) node {
	attrs := node{
		"level": level,
		"style": fmt.Sprintf("color:%s;font-family:%s", b.kit.TextColor, b.kit.HeadingFont),
	}
	return element("heading", attrs, text(s))
}

func (b brand) paragraph(s string) node {
	return element("paragraph", styled(fmt.Sprintf("color:%s;font-family:%s", b.kit.TextColor, b.kit.BodyFont)), text(s))
}

func (b brand) footer() node {
	style := fmt.Sprintf("color:%s;font-family:%s;font-size:12px", b.kit.TextColor, b.kit.BodyFont)
	return element("paragraph", styled(style), text(b.kit.FooterText))
}

func (b brand) button(label string) node {
	attrs := node{
		"href":      "https://example.com",
		"alignment": "left",
		"style": fmt.Sprintf("background-color:%s;color:%s;border-radius:%dpx;font-family:%s",
			b.kit.AccentColor, b.kit.ButtonTextColor, b.kit.ButtonRadius, b.kit.BodyFont),
	}
	return element("button", attrs, text(label))
}

func (b brand) rule() node {
	return element("horizontalRule", nil)
}

// logo is the kit's logo as an image, or nil when it has none.
func (b brand) logo() node {
	if b.kit.LogoURL == "" {
		return nil
	}
	attrs := node{"src": b.kit.LogoURL, "alt": i18n.T(i18n.BrandkitLogoAlt, nil), "alignment": "left", "width": "120", "height": "auto"}
	return element("image", attrs)
}
