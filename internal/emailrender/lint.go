package emailrender

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Code is what a lint finding is about.
type Code string

const (
	// CodeInvalidContent is the only error: the subject or body cannot render.
	CodeInvalidContent Code = "invalid_content"
	// CodeMissingAlt: an <img> has no alt attribute (alt="" is a decorative image and is fine).
	CodeMissingAlt Code = "missing_alt"
	// CodeLowContrast: text is below the WCAG AA contrast ratio against its background.
	CodeLowContrast Code = "low_contrast"
	// CodeOffBrandColor: a color is not one of the Brand kit's.
	CodeOffBrandColor Code = "off_brand_color"
	// CodeOffBrandFont: a font is not one of the Brand kit's.
	CodeOffBrandFont Code = "off_brand_font"
	// CodeUnknownConstruct: an element the sanitizer removes on save.
	CodeUnknownConstruct Code = "unknown_construct"
)

// Finding is one result of Lint. Message is fixed text of the server; Element and Detail quote
// the message or the Brand kit and are untrusted data wherever an agent reads them.
type Finding struct {
	Code    Code
	Message string
	// Element is a short description of the element, such as `<img src="…">`.
	Element string
	// Detail is the offending value or the reason, such as `#ff0000` or `3.2:1, needs 4.5:1`.
	Detail string
}

// Brand is what Lint compares a message's colors and fonts to: the Workspace's Brand kit.
// A nil *Brand (no Brand kit saved) switches the brand warnings off.
type Brand struct {
	// Colors are the kit's colors; a message may use these.
	Colors []string
	// Fonts are the kit's font-family lists; a message's first family must be the first
	// family of one of them.
	Fonts []string
	// TextColor and BackgroundColor are what text and a page get when the message names none,
	// for the contrast check.
	TextColor, BackgroundColor string
}

// Report is the result of Lint over one message.
type Report struct {
	// Error is non-nil only when the message cannot render (ErrInvalidContent wrapped): it
	// blocks a save and a send.
	Error error
	// Warnings are everything else; they never block anything.
	Warnings []Finding
}

// maxWarnings caps a report: a pathological message must not return thousands of findings.
const maxWarnings = 50

// Lint is the one lint of a message (ADR 0040), over its rendered HTML: the save paths and
// the send planner use its Error, the editor's responses and the MCP tool email_lint return its
// Warnings too. Error is a Liquid syntax error or HTML that does not parse; the warnings are a
// missing image description, low contrast, a color or font outside the Brand kit, and an
// element the sanitizer removes. It runs on the message as the author wrote it, before
// SanitizeHTML (sanitizing would repair what it should refuse and hide what it should name),
// and with no recipient data: unbound merge tags render empty.
//
// Message weight is deliberately not checked: no first-party source gives a size at which a mail
// client clips or penalizes a message (the only figure found is Beefree's own 80 KB setting).
//
// Contrast follows WCAG 2.2 success criterion 1.4.3 (Contrast (Minimum), Level AA): 4.5:1 for
// text, 3:1 for large text (at least 18 pt, or 14 pt bold), with the relative luminance and
// contrast ratio formulas of https://www.w3.org/TR/WCAG22/#dfn-relative-luminance and
// https://www.w3.org/TR/WCAG22/#dfn-contrast-ratio.
func Lint(subject, body string, brand *Brand) Report {
	_, rendered, err := renderBoth(subject, body, map[string]any{})
	if err != nil {
		return Report{Error: fmt.Errorf("%w: %w", ErrInvalidContent, err)}
	}
	if err := checkHTML(rendered); err != nil {
		return Report{Error: fmt.Errorf("%w: %w", ErrInvalidContent, err)}
	}
	root, err := html.Parse(strings.NewReader(rendered))
	if err != nil {
		return Report{Error: fmt.Errorf("%w: %w", ErrInvalidContent, err)}
	}
	l := linter{brand: brand.normalized(), seen: map[Finding]bool{}}
	l.walk(root)
	return Report{Warnings: l.found}
}

type linter struct {
	brand *Brand
	found []Finding
	seen  map[Finding]bool
}

func (l *linter) add(f Finding) {
	if len(l.found) >= maxWarnings || l.seen[f] {
		return
	}
	l.seen[f] = true
	l.found = append(l.found, f)
}

var knownElements = func() map[string]bool {
	m := map[string]bool{"style": true}
	for _, name := range slices.Concat(emailElements, vmlElements) {
		m[name] = true
	}
	return m
}()

func (l *linter) walk(n *html.Node) {
	if n.Type == html.ElementNode {
		l.element(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		l.walk(c)
	}
}

func (l *linter) element(n *html.Node) {
	el := describe(n)
	if !knownElements[n.Data] {
		l.add(Finding{Code: CodeUnknownConstruct, Message: "this element is removed when the message is saved", Element: el})
	}
	if n.Data == "img" && !hasAttr(n, "alt") {
		l.add(Finding{Code: CodeMissingAlt, Message: "this image has no alt attribute", Element: el})
	}
	if l.brand != nil {
		l.brandCheck(n, el)
	}
	if hasOwnText(n) {
		l.contrast(n, el)
	}
}

func (l *linter) brandCheck(n *html.Node, el string) {
	text := hasOwnText(n)
	for _, c := range colorsOf(n, text) {
		if !slices.Contains(l.brand.Colors, c) && !neutral[c] {
			l.add(Finding{Code: CodeOffBrandColor, Message: "this color is not one of the Brand kit's colors", Element: el, Detail: c})
		}
	}
	if font := fontOf(n); text && font != "" && !slices.Contains(l.brand.Fonts, firstFamily(font)) {
		l.add(Finding{Code: CodeOffBrandFont, Message: "this font is not one of the Brand kit's fonts", Element: el, Detail: font})
	}
}

func (l *linter) contrast(n *html.Node, el string) {
	fg, fgOK := inherited(n, "color", func(m *html.Node) string { return attr(m, "color") })
	bg, bgOK := inherited(n, "background-color", func(m *html.Node) string { return attr(m, "bgcolor") })
	if bg == "" && bgOK {
		bg, bgOK = inherited(n, "background", nil)
	}
	switch {
	case fg == "" && l.brand != nil:
		fg, fgOK = l.brand.TextColor, true
	case fg == "":
		fg, fgOK = "#000000", true
	}
	switch {
	case bg == "" && l.brand != nil:
		bg, bgOK = l.brand.BackgroundColor, true
	case bg == "":
		bg, bgOK = "#ffffff", true
	}
	if !fgOK || !bgOK {
		return
	}
	f, ok1 := parseColor(fg)
	b, ok2 := parseColor(bg)
	if !ok1 || !ok2 {
		return
	}
	need, label := 4.5, "4.5:1"
	if isLarge(n) {
		need, label = 3, "3:1"
	}
	if ratio := contrastRatio(f, b); ratio < need {
		l.add(Finding{
			Code: CodeLowContrast, Message: "the text and its background are below the WCAG AA contrast ratio", Element: el,
			Detail: fmt.Sprintf("%.1f:1, needs %s", ratio, label),
		})
	}
}

// inherited returns the value of the CSS property on n or its nearest ancestor that sets it
// (falling back to the legacy HTML attribute when legacy is not nil), normalized. A "transparent"
// value is skipped. ok is false when the nearest value is not a plain color the check can use.
func inherited(n *html.Node, prop string, legacy func(*html.Node) string) (string, bool) {
	for m := n; m != nil; m = m.Parent {
		if m.Type != html.ElementNode {
			continue
		}
		v := declared(m)[prop]
		if v == "" && legacy != nil {
			v = legacy(m)
		}
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || v == "transparent" || v == "inherit" || v == "initial" {
			continue
		}
		c, ok := normalizeColor(v)
		if prop == "background" && !ok {
			return "", false
		}
		return c, ok
	}
	return "", true
}

// declared parses the element's inline style into property -> value (lower-case property).
func declared(n *html.Node) map[string]string {
	out := map[string]string{}
	style := attr(n, "style")
	if style == "" {
		return out
	}
	for _, decl := range splitDeclarations(style) {
		if prop, value, ok := strings.Cut(decl, ":"); ok {
			out[strings.ToLower(strings.TrimSpace(prop))] = strings.TrimSpace(value)
		}
	}
	return out
}

// neutral colors are allowed in any message: white and black belong to no brand.
var neutral = map[string]bool{"#ffffff": true, "#000000": true}

var colorToken = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b|rgba?\([^)]*\)|\b(?:white|black|red|green|blue|gray|grey|yellow|orange|purple|silver|navy|teal|maroon|olive|lime|aqua|fuchsia)\b`)

// colorsOf lists the normalized colors an element sets: its text color when it has text of its
// own (a wrapper's color is only inherited by what it holds), its background declarations and
// bgcolor. Border colors are decoration and are not checked.
func colorsOf(n *html.Node, text bool) []string {
	var out []string
	add := func(value string) {
		for _, tok := range colorToken.FindAllString(value, -1) {
			if c, ok := normalizeColor(strings.ToLower(tok)); ok && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	decl := declared(n)
	for _, prop := range slices.Sorted(maps.Keys(decl)) {
		if (prop == "color" && text) || strings.HasPrefix(prop, "background") {
			add(decl[prop])
		}
	}
	add(attr(n, "bgcolor"))
	if n.Data == "font" && text {
		add(attr(n, "color"))
	}
	return out
}

// splitDeclarations splits an inline style on the semicolons outside parentheses and quotes.
// (The CSS parser the project already depends on, douceur, drops the value of a declaration
// that holds a comma, such as a font-family list, so it cannot be used here.)
func splitDeclarations(style string) []string {
	var out []string
	depth, start := 0, 0
	var quote rune
	for i, r := range style {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth = max(depth-1, 0)
		case r == ';' && depth == 0:
			out = append(out, style[start:i])
			start = i + 1
		}
	}
	return append(out, style[start:])
}

// fontOf is the element's own font-family list (style, or <font face>).
func fontOf(n *html.Node) string {
	if v := declared(n)["font-family"]; v != "" {
		return v
	}
	if n.Data == "font" {
		return attr(n, "face")
	}
	return ""
}

// firstFamily is the first family of a font-family list: lower-case, unquoted.
func firstFamily(list string) string {
	first, _, _ := strings.Cut(list, ",")
	return strings.ToLower(strings.Trim(strings.TrimSpace(first), `"'`))
}

// isLarge is WCAG's large text: at least 18 pt (24 px), or 14 pt (18.66 px) and bold, from the
// element's own or an ancestor's font-size in px or pt.
func isLarge(n *html.Node) bool {
	size := 0.0
	bold := false
	for m := n; m != nil && (size == 0 || !bold); m = m.Parent {
		if m.Type != html.ElementNode {
			continue
		}
		d := declared(m)
		if size == 0 {
			size = pixels(d["font-size"])
		}
		if w := d["font-weight"]; !bold && (w == "bold" || w == "bolder" || weightAtLeast700(w)) {
			bold = true
		}
		if m.Data == "b" || m.Data == "strong" || (len(m.Data) == 2 && m.Data[0] == 'h' && m.Data[1] >= '1' && m.Data[1] <= '6') {
			bold = true
		}
	}
	return size >= 24 || (bold && size >= 18.66)
}

func weightAtLeast700(w string) bool {
	n, err := strconv.Atoi(w)
	return err == nil && n >= 700
}

func pixels(v string) float64 {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case strings.HasSuffix(v, "px"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
		return f
	case strings.HasSuffix(v, "pt"):
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "pt"), 64)
		return f * 4 / 3
	}
	return 0
}

func hasOwnText(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

// describe names an element for the author: its tag, the attribute that identifies it (src of
// an image, href of a link) or the start of its own text.
func describe(n *html.Node) string {
	switch n.Data {
	case "img":
		return fmt.Sprintf(`<img src=%q>`, clip(attr(n, "src"), 80))
	case "a":
		return fmt.Sprintf(`<a href=%q>`, clip(attr(n, "href"), 80))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			if t := strings.Join(strings.Fields(c.Data), " "); t != "" {
				return fmt.Sprintf("<%s> %q", n.Data, clip(t, 40))
			}
		}
	}
	return "<" + n.Data + ">"
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

var (
	namedColors = map[string]string{"white": "#ffffff", "black": "#000000"}
	colorWord   = regexp.MustCompile(`^[a-z]+$`)
	rgbColor    = regexp.MustCompile(`^rgba?\(\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*[, ]\s*(\d{1,3})\s*(?:[,/]\s*[\d.]+%?\s*)?\)$`)
)

// normalizeColor turns #rgb, #rrggbb, rgb(…) and white/black into #rrggbb. Other named
// colors stay as the name, so they compare equal only to themselves; ok is false for anything
// that is not a color value at all (a keyword such as currentcolor).
func normalizeColor(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if c, ok := namedColors[v]; ok {
		return c, true
	}
	if m := rgbColor.FindStringSubmatch(v); m != nil {
		var rgb [3]int
		for i := range rgb {
			rgb[i], _ = strconv.Atoi(m[i+1])
			if rgb[i] > 255 {
				return "", false
			}
		}
		return fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2]), true
	}
	if strings.HasPrefix(v, "#") {
		switch len(v) {
		case 4:
			return "#" + strings.Repeat(string(v[1]), 2) + strings.Repeat(string(v[2]), 2) + strings.Repeat(string(v[3]), 2), isHex(v[1:])
		case 7:
			return v, isHex(v[1:])
		}
		return "", false
	}
	if colorWord.MatchString(v) && v != "currentcolor" {
		return v, true
	}
	return "", false
}

func isHex(s string) bool {
	_, err := strconv.ParseUint(s, 16, 32)
	return err == nil
}

// parseColor reads a normalized #rrggbb; named colors other than white and black are unknown.
func parseColor(c string) ([3]float64, bool) {
	if len(c) != 7 || c[0] != '#' {
		return [3]float64{}, false
	}
	var out [3]float64
	for i := range out {
		v, err := strconv.ParseUint(c[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return out, false
		}
		out[i] = float64(v) / 255
	}
	return out, true
}

func luminance(c [3]float64) float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

func contrastRatio(a, b [3]float64) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// normalized returns a copy of the brand with its values in the form the message's values are
// normalized to, or nil for no brand.
func (b *Brand) normalized() *Brand {
	if b == nil {
		return nil
	}
	out := &Brand{}
	for _, c := range b.Colors {
		if n, ok := normalizeColor(c); ok {
			out.Colors = append(out.Colors, n)
		}
	}
	for _, f := range b.Fonts {
		out.Fonts = append(out.Fonts, firstFamily(f))
	}
	out.TextColor, _ = normalizeColor(b.TextColor)
	out.BackgroundColor, _ = normalizeColor(b.BackgroundColor)
	return out
}
