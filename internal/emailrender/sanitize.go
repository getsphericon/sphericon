package emailrender

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// SanitizeHTML is the sanitizer of ADR 0040: it strips script, frames, forms, event-handler
// attributes, dangerous URLs and dangerous CSS from an email body and keeps what email
// markup needs (tables, inline styles, style blocks, Outlook conditional comments and VML
// buttons). It runs where a body is saved and again on each recipient's rendered message
// (RenderEmail), so Liquid merge tags in the input are kept byte for byte: they are masked
// while the policy runs. Applying it to its own output changes nothing.
func SanitizeHTML(src string) string { return sanitize(src, true) }

// sanitizeRendered is the second pass: the message after Liquid has run holds no merge
// tags, so a "{{" left in it is text a variable carried, and nothing is masked.
func sanitizeRendered(src string) string { return sanitize(src, false) }

// Allowed markup. bluemonday is tokenizer-based (it never foster-parents), so it keeps
// Liquid tags between table rows where they stand.
var (
	bodyPolicy = newPolicy(false)
	vmlPolicy  = newPolicy(true)
)

// liquidTok is a masked Liquid tag (see pass): a merge tag may stand for a whole style
// attribute or the start of a url() while the policy runs. Its value is checked after
// Liquid has run, in the second pass.
const liquidTok = `sph[0-9a-f]{16}L\d+x`

var (
	// Plain tokens in attribute and CSS values can only be made of these characters. The
	// style attribute is checked as a whole against an allowlist: declarations whose
	// values hold words, numbers, colours, rgb()/hsl()/calc() and url() of http(s) or a
	// relative path. No ':' (so no javascript:), no backslash or '&' (no escape
	// obfuscation), no '*' (no comments), no other function.
	cssWord  = `[\p{L}\p{N}#%.,_!'"/\s+-]`
	cssFunc  = `(?:rgba?|hsla?|calc|var)\([\p{L}\p{N} .,%+*/_-]*\)`
	cssURL   = `url\(\s*['"]?(?:https?://[^\s)'"\\<>]*|` + liquidTok + `[^\s)'":\\<>]*)['"]?\s*\)`
	cssDecl  = `\s*-{0,2}[A-Za-z][A-Za-z-]*\s*:\s*(?:` + cssWord + `|` + cssFunc + `|` + cssURL + `)+\s*`
	cssItem  = `(?:` + cssDecl + `|\s*` + liquidTok + `\s*)`
	inlineCS = regexp.MustCompile(`(?i)^(?:` + cssItem + `;)*(?:` + cssItem + `)?;?\s*$`)

	// The href of a VML element is not checked by bluemonday's URL schemes.
	vmlHref = regexp.MustCompile(`(?i)^(?:(?:https?://|mailto:|tel:)[^\s<>"'\\]*|[^\s:<>"'\\]*)$`)

	condText    = `[A-Za-z0-9 !&|()<>=.]{1,60}`
	hiddenCond  = regexp.MustCompile(`(?s)^\[if (` + condText + `)\]>(.*)<!\[endif\]$`)
	revealOpen  = regexp.MustCompile(`^\[if (` + condText + `)\]><!(?:--)?\s*$`)
	doctypeHTML = regexp.MustCompile(`(?i)^<!doctype html(?: public "[^"<>]*"(?: "[^"<>]*")?)?\s*>$`)
	// The Outlook settings block is case-sensitive XML, which the policy would lower-case,
	// so it is kept only when it is exactly this shape.
	officeSettings = regexp.MustCompile(`^\s*<xml>\s*<o:OfficeDocumentSettings>\s*(?:<o:AllowPNG\s*/>\s*)?(?:<o:PixelsPerInch>\d{1,4}</o:PixelsPerInch>\s*)?</o:OfficeDocumentSettings>\s*</xml>\s*$`)
	mediaQuery     = regexp.MustCompile(`^[A-Za-z0-9 ,:()._-]{1,200}$`)
)

func newPolicy(vml bool) *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"html", "head", "meta", "title", "body",
		"table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "caption",
		"div", "span", "p", "br", "hr", "center", "font", "blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li",
		"b", "strong", "i", "em", "u", "s", "small", "sub", "sup", "a", "img",
	)
	p.AllowAttrs("class", "id", "lang", "dir", "title", "role", "align", "valign", "width", "height",
		"bgcolor", "border", "cellpadding", "cellspacing", "colspan", "rowspan", "color", "face", "size",
		"aria-label", "aria-hidden").Globally()
	p.AllowAttrs("style").Matching(inlineCS).Globally()
	p.AllowDataAttributes()
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_(?:blank|self)$`)).OnElements("a")
	p.AllowAttrs("src", "alt").OnElements("img")
	p.AllowAttrs("name", "content", "charset").OnElements("meta")
	p.AllowAttrs("xmlns").Matching(regexp.MustCompile(`^http://www\.w3\.org/1999/xhtml$`)).OnElements("html")
	p.AllowAttrs("xmlns:v").Matching(regexp.MustCompile(`^urn:schemas-microsoft-com:vml$`)).OnElements("html", "v:roundrect", "v:rect")
	p.AllowAttrs("xmlns:o").Matching(regexp.MustCompile(`^urn:schemas-microsoft-com:office:office$`)).OnElements("html")
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid")
	p.AllowRelativeURLs(true)
	p.RequireParseableURLs(true)
	p.SkipElementsContent("applet", "frame", "frameset", "iframe", "noembed", "noframes", "noscript", "nostyle",
		"object", "script", "style", "xmp", "plaintext", "textarea", "template")
	if !vml {
		return p
	}
	p.AllowElements("v:roundrect", "v:rect", "v:oval", "v:line", "v:shape", "v:image", "v:fill", "v:stroke",
		"v:textbox", "v:background", "w:anchorlock")
	p.AllowNoAttrs().OnElements("v:fill", "v:textbox", "v:background", "w:anchorlock")
	p.AllowAttrs("arcsize", "strokecolor", "strokeweight", "fillcolor", "stroke", "fill", "type", "color", "origin",
		"position", "inset", "o:title", "o:spid", "coordsize", "from", "to", "opacity", "aspect", "size").
		OnElements("v:roundrect", "v:rect", "v:oval", "v:line", "v:shape", "v:image", "v:fill", "v:stroke", "v:textbox", "v:background")
	p.AllowAttrs("href").Matching(vmlHref).OnElements("v:roundrect", "v:rect", "v:oval", "v:shape", "v:image")
	p.AllowAttrs("src").OnElements("v:fill", "v:image", "v:background")
	return p
}

// Lifted pieces (comments, style blocks, a doctype) and Liquid tags are replaced by tokens
// made of letters and digits while bluemonday runs: a token survives text, attribute
// values and URLs unchanged. Each call draws its own nonce, so markup in the input cannot
// forge a token.
var tokenRe = regexp.MustCompile(`sph[0-9a-f]{16}([LCSD])(\d+)x`)

var liquidRe = regexp.MustCompile(`(?s)\{\{.*?\}\}|\{%.*?%\}`)

type pass struct {
	prefix string
	pieces map[string]string
	n      int
}

func newPass() *pass {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return &pass{prefix: "sph" + hex.EncodeToString(b[:]), pieces: map[string]string{}}
}

func (p *pass) token(kind byte, content string) string {
	tok := p.prefix + string(kind) + strconv.Itoa(p.n) + "x"
	p.n++
	p.pieces[tok] = content
	return tok
}

// restore puts every piece of this pass back. A piece can hold tokens of its own (a
// conditional comment's inner HTML holds lifted styles and Liquid tags), hence the loop;
// nesting is one level deep, so it ends.
func (p *pass) restore(s string) string {
	for range 4 {
		changed := false
		s = tokenRe.ReplaceAllStringFunc(s, func(tok string) string {
			content, ok := p.pieces[tok]
			if ok {
				changed = true
				return content
			}
			return tok
		})
		if !changed {
			break
		}
	}
	return s
}

func sanitize(src string, maskLiquid bool) string {
	p := newPass()
	if maskLiquid {
		src = liquidRe.ReplaceAllStringFunc(src, func(tag string) string { return p.token('L', tag) })
	}
	return p.restore(p.clean(src, false))
}

// clean lifts comments, style blocks and the doctype out of src (bluemonday drops all
// three), runs the policy over the rest and returns the result with tokens in place of
// the lifted pieces. nested is the inner HTML of a conditional comment: it has no
// conditional comments of its own.
func (p *pass) clean(src string, nested bool) string {
	var out strings.Builder
	z := html.NewTokenizer(strings.NewReader(src))
loop:
	for {
		switch z.Next() {
		case html.ErrorToken:
			out.Write(z.Raw())
			break loop
		case html.CommentToken:
			out.WriteString(p.comment(z.Token().Data, nested))
		case html.DoctypeToken:
			if raw := string(z.Raw()); !nested && doctypeHTML.MatchString(raw) {
				out.WriteString(p.token('D', raw))
			}
		case html.StartTagToken:
			tag := z.Token()
			if tag.Data != "style" {
				out.Write(z.Raw())
				continue
			}
			out.WriteString(p.style(z, tag))
		default:
			out.Write(z.Raw())
		}
	}
	policy := bodyPolicy
	if nested {
		policy = vmlPolicy
	}
	return policy.Sanitize(out.String())
}

// comment keeps the three forms of Outlook conditional comment and drops every other
// comment. The condition is checked against an allowlist and the HTML a hidden
// conditional hides is sanitized like the rest, with the VML elements allowed.
func (p *pass) comment(data string, nested bool) string {
	if nested {
		return ""
	}
	if data == "<![endif]" {
		return p.token('C', "<!--<![endif]-->")
	}
	if m := revealOpen.FindStringSubmatch(data); m != nil {
		return p.token('C', "<!--[if "+m[1]+"]><!-->")
	}
	if m := hiddenCond.FindStringSubmatch(data); m != nil {
		inner := m[2]
		if !officeSettings.MatchString(inner) {
			inner = p.clean(inner, true)
		}
		return p.token('C', "<!--[if "+m[1]+"]>"+inner+"<![endif]-->")
	}
	return ""
}

// style lifts a style block. The block is kept only whole and only when its CSS passes
// safeCSS; its media attribute is the one attribute kept. The tokenizer reads the
// content of a style element as one raw text, so "</style><script>" cannot end it early
// inside this pass and then reach the policy as markup.
func (p *pass) style(z *html.Tokenizer, tag html.Token) string {
	var css strings.Builder
	closed := false
	for !closed {
		switch z.Next() {
		case html.TextToken:
			css.Write(z.Raw())
		case html.EndTagToken:
			closed = true
		default:
			return ""
		}
	}
	if !safeCSS(css.String()) {
		return ""
	}
	open := "<style>"
	for _, a := range tag.Attr {
		if a.Key == "media" && mediaQuery.MatchString(a.Val) {
			open = `<style media="` + a.Val + `">`
		}
	}
	return p.token('S', open+css.String()+"</style>")
}

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssURLArg  = regexp.MustCompile(`(?i)url\(([^)]*)\)`)
	cssBanned  = []string{"@import", "expression", "javascript:", "vbscript:", "behavior", "binding", "data:", "<"}
	blankChars = regexp.MustCompile(`[\s\x00-\x1f]+`)
)

// safeCSS reports whether the CSS of a style block may be kept. It is a denylist, not a
// CSS parse: no escapes, no markup, no @import, no script-ish values, and every url()
// is http(s) or relative.
func safeCSS(css string) bool {
	if strings.ContainsAny(css, "\\<\x00") {
		return false
	}
	css = strings.ToLower(cssComment.ReplaceAllString(css, " "))
	for _, banned := range cssBanned {
		if strings.Contains(css, banned) {
			return false
		}
	}
	for _, m := range cssURLArg.FindAllStringSubmatch(css, -1) {
		arg := strings.Trim(blankChars.ReplaceAllString(m[1], ""), `'"`)
		if strings.Contains(arg, ":") && !strings.HasPrefix(arg, "http://") && !strings.HasPrefix(arg, "https://") {
			return false
		}
	}
	return true
}
