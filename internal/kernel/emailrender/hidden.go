package emailrender

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// notRendered are the elements whose text no reader sees in the body.
var notRendered = map[atom.Atom]bool{
	atom.Head: true, atom.Title: true, atom.Style: true, atom.Script: true, atom.Template: true,
}

// hiddenContent warns about text the reader cannot see (ADR 0188): an element hidden by inline
// style that holds text, other than the preheader and a block a media query of the message shows,
// and invisible characters in the subject and the visible text.
func (l *linter) hiddenContent(subject string, root *html.Node) {
	l.characters(subject, fmt.Sprintf("subject %q", clip(subject, 40)))
	l.visibleTree(root, preheader(root), shownClasses(root))
}

// visibleTree walks what the reader sees under n; a hidden element is reported and not entered,
// and the preheader is left out. An element with a class in shown is visible.
func (l *linter) visibleTree(n, pre *html.Node, shown map[string]bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode && n.Type == html.ElementNode:
			l.characters(c.Data, describe(n))
		case c == pre || c.Type != html.ElementNode || notRendered[c.DataAtom]:
		case hiddenByStyle(c) && !slices.ContainsFunc(strings.Fields(attr(c, "class")), func(class string) bool { return shown[class] }):
			if text := textOf(c); text != "" {
				l.add(Finding{
					Code: CodeHiddenText, Message: "this element is hidden from the reader but holds text",
					Element: describe(c), Detail: clip(text, 40),
				})
			}
		default:
			l.visibleTree(c, pre, shown)
		}
	}
}

// characters warns when text holds an invisible character used to hide text (hiddenCharacters);
// el names where the text is.
func (l *linter) characters(text, el string) {
	found := hiddenCharacters(text)
	if len(found) == 0 {
		return
	}
	points := make([]string, 0, len(found))
	for _, r := range found {
		points = append(points, fmt.Sprintf("U+%04X", r))
	}
	l.add(Finding{
		Code: CodeHiddenCharacters, Message: "this text holds an invisible character, which filters read as hidden words",
		Element: el, Detail: strings.Join(points, " "),
	})
}

// The characters of an emoji flag's tag sequence (Unicode Technical Standard #51): the black
// flag, tag characters, and the cancel tag that ends it.
const (
	blackFlag = '\U0001F3F4'
	cancelTag = '\U000E007F'
)

// hiddenCharacters lists, once each and in order, the invisible characters of text that hide
// words: a Unicode tag character (U+E0000 to U+E007F) outside an emoji flag's tag sequence, and
// the zero-width space, word joiner and zero-width no-break space (U+200B, U+2060, U+FEFF). The
// joiners of emoji and of scripts (U+200C, U+200D) and the direction marks are never listed, nor
// the zero-width space of the editor's code block, which writes each space of code as U+00A0
// U+200D U+200B.
func hiddenCharacters(text string) []rune {
	runes := []rune(text)
	var out []rune
	add := func(r rune) {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == blackFlag:
			if end := flagEnd(runes, i); end > i {
				i = end
			}
		case r >= 0xE0000 && r <= cancelTag:
			add(r)
		case r == '\u200b':
			if i < 2 || runes[i-1] != '\u200d' || runes[i-2] != '\u00a0' {
				add(r)
			}
		case r == '\u2060' || r == '\ufeff':
			add(r)
		}
	}
	return out
}

// flagEnd is the index of the cancel tag that ends the flag sequence starting at the black flag
// at i: one or more tag characters U+E0020 to U+E007E, then U+E007F. It is i when none does.
func flagEnd(runes []rune, i int) int {
	j := i + 1
	for j < len(runes) && runes[j] >= 0xE0020 && runes[j] <= 0xE007E {
		j++
	}
	if j == i+1 || j >= len(runes) || runes[j] != cancelTag {
		return i
	}
	return j
}

// textOf is the text n holds, its whitespace collapsed; "" when it holds none.
func textOf(n *html.Node) string {
	var words []string
	var collect func(*html.Node)
	collect = func(m *html.Node) {
		for c := m.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode:
				words = append(words, strings.FieldsFunc(c.Data, isFiller)...)
			case c.Type == html.ElementNode && !notRendered[c.DataAtom]:
				collect(c)
			}
		}
	}
	collect(n)
	return strings.Join(words, " ")
}

// preheader is the message's preview text, which a mail client shows after the subject in the
// inbox and the reader never sees in the message: the first element hidden by inline style
// (hiddenByStyle) that comes before any visible content of the message, with everything inside
// it (the padding of invisible characters that follows the text). Visible content is a text node
// with a character other than filler, or an image, outside the head, <title>, <style>, <script>
// and <template>. It is nil when visible content comes first or nothing is hidden.
func preheader(root *html.Node) *html.Node {
	found, _ := firstHidden(root)
	return found
}

// firstHidden looks under n, in document order, for a hidden element before visible content;
// done is true once either is met.
func firstHidden(n *html.Node) (found *html.Node, done bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode && strings.IndexFunc(c.Data, isNotFiller) >= 0:
			return nil, true
		case c.Type != html.ElementNode || notRendered[c.DataAtom]:
			continue
		case hiddenByStyle(c):
			return c, true
		case c.DataAtom == atom.Img:
			return nil, true
		}
		if found, done := firstHidden(c); done {
			return found, true
		}
	}
	return nil, false
}

// isFiller is a character that shows nothing on its own: white space, and the invisible
// characters a preheader is padded with (the zero-width space, joiners, word joiner, zero-width
// no-break space, combining grapheme joiner and soft hyphen). Text made only of filler is no text.
func isFiller(r rune) bool {
	switch r {
	case '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff', '\u034f', '\u00ad':
		return true
	}
	return unicode.IsSpace(r)
}

func isNotFiller(r rune) bool { return !isFiller(r) }

// shownClasses are the classes a rule inside an @media block of the message's <style> elements
// shows: the rule sets a hiding property (hidingDeclarations, max-height) to a value that does not
// hide, as a block for small screens only does.
func shownClasses(root *html.Node) map[string]bool {
	shown := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.DataAtom == atom.Style {
				addShownClasses(shown, textOfStyle(c))
				continue
			}
			visit(c)
		}
	}
	visit(root)
	return shown
}

func textOfStyle(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

// addShownClasses reads one stylesheet with tdewolff/parse's CSS parser.
func addShownClasses(shown map[string]bool, sheet string) {
	var atRules []bool // per open at-rule block, whether it is @media
	var classes []string
	p := css.NewParser(parse.NewInputString(sheet), false)
	for {
		gt, _, data := p.Next()
		switch gt {
		case css.ErrorGrammar:
			if !p.HasParseError() {
				return // the end of the sheet
			}
		case css.BeginAtRuleGrammar:
			atRules = append(atRules, strings.EqualFold(string(data), "@media"))
		case css.EndAtRuleGrammar:
			if len(atRules) > 0 {
				atRules = atRules[:len(atRules)-1]
			}
		case css.BeginRulesetGrammar:
			classes = nil
			if slices.Contains(atRules, true) {
				classes = classesIn(p.Values())
			}
		case css.EndRulesetGrammar:
			classes = nil
		case css.DeclarationGrammar:
			if len(classes) > 0 && shows(strings.ToLower(string(data)), p.Values()) {
				for _, class := range classes {
					shown[class] = true
				}
			}
		default:
		}
	}
}

// classesIn lists the classes a selector names (each "." followed by a name).
func classesIn(selector []css.Token) []string {
	var out []string
	for i := 0; i+1 < len(selector); i++ {
		if selector[i].TokenType == css.DelimToken && string(selector[i].Data) == "." && selector[i+1].TokenType == css.IdentToken {
			out = append(out, string(selector[i+1].Data))
		}
	}
	return out
}

// shows reports whether a declaration sets a property that can hide an element to a value that
// does not.
func shows(prop string, values []css.Token) bool {
	values, _ = withoutImportant(values)
	var sb strings.Builder
	for _, t := range values {
		sb.Write(t.Data)
	}
	value := strings.ToLower(strings.TrimSpace(sb.String()))
	if hiding, ok := hidingDeclarations[prop]; ok {
		return !slices.Contains(hiding, value)
	}
	return prop == "max-height" && !slices.Contains(zeroLengths, value)
}
