package emailrender

import (
	"slices"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
)

// hidingDeclarations are the inline style declarations (property, value) mail authors use to
// hide an element from the reader while keeping it in the HTML. Only inline styles are read:
// hiding through a class or a <style> rule is not detected. A zero font size is not one of
// them: it is how a layout table removes the gaps between its columns, and the text inside
// sets its own size.
var hidingDeclarations = map[string][]string{
	"display":    {"none"},
	"visibility": {"hidden"},
	"mso-hide":   {"all"},
	"opacity":    {"0"},
}

// zeroLengths are the values of a zero max-height.
var zeroLengths = []string{"0", "0px", "0pt", "0em"}

// hidesElement reports whether an inline style hides its element: one of the hiding
// declarations, or a zero max-height that clips its content (overflow:hidden), the shape of a
// preheader. A zero max-height alone clips nothing. The values are compared lower-case, and a
// declaration whose value its property does not take is dropped (takesValue).
func hidesElement(style string) bool {
	parsed := parseStyle(style, takesValue)
	for prop, hiding := range hidingDeclarations {
		value := strings.ToLower(parsed[prop])
		if slices.Contains(hiding, value) {
			return true
		}
	}
	maxHeight, overflow := strings.ToLower(parsed["max-height"]), strings.ToLower(parsed["overflow"])
	return slices.Contains(zeroLengths, maxHeight) && overflow == "hidden"
}

// dropHidden removes, in place, every element whose inline style hides it.
func dropHidden(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && slices.ContainsFunc(c.Attr, func(a html.Attribute) bool {
			return a.Key == "style" && hidesElement(a.Val)
		}) {
			n.RemoveChild(c)
		} else {
			dropHidden(c)
		}
		c = next
	}
}

// takenValues tells, for each property the hiding check reads, whether it takes a value (its
// tokens, whitespace dropped, idents lower-case). A browser drops a declaration whose value its
// property does not take, so display:none;display:bogus still hides.
var takenValues = map[string]func([]css.Token) bool{
	"display": func(v []css.Token) bool {
		return (len(v) == 1 && identIn(v[0], displayKeywords...)) ||
			(len(v) >= 2 && len(v) <= 3 && allIdentsIn(v, displayParts))
	},
	"visibility": func(v []css.Token) bool { return len(v) == 1 && identIn(v[0], "visible", "hidden", "collapse") },
	"mso-hide":   func(v []css.Token) bool { return len(v) == 1 && identIn(v[0], "all", "none") },
	"opacity": func(v []css.Token) bool {
		return len(v) == 1 && (v[0].TokenType == css.NumberToken || v[0].TokenType == css.PercentageToken)
	},
	"max-height": func(v []css.Token) bool {
		return len(v) == 1 && (identIn(v[0], "none", "min-content", "max-content", "fit-content") ||
			v[0].TokenType == css.PercentageToken || isLength(v[0]) || isZero(v[0]))
	},
	"overflow": func(v []css.Token) bool {
		return len(v) >= 1 && len(v) <= 2 && allIdentsIn(v, overflowKeywords)
	},
}

// displayKeywords are the one-word values of display; displayParts the words of its multi-keyword
// form (block flow, inline flex, block flow list-item, ...).
var (
	displayKeywords = []string{
		"none", "contents", "block", "inline", "inline-block", "flow-root", "list-item", "run-in",
		"flex", "inline-flex", "grid", "inline-grid", "table", "inline-table", "table-row-group",
		"table-header-group", "table-footer-group", "table-row", "table-cell", "table-column-group",
		"table-column", "table-caption", "ruby", "ruby-base", "ruby-text", "ruby-base-container",
		"ruby-text-container", "-webkit-box", "-webkit-inline-box", "-ms-flexbox", "-ms-inline-flexbox",
	}
	displayParts     = []string{"block", "inline", "run-in", "flow", "flow-root", "table", "flex", "grid", "ruby", "list-item"}
	overflowKeywords = []string{"visible", "hidden", "clip", "scroll", "auto", "overlay"}
	cssWideKeywords  = []string{"inherit", "initial", "unset", "revert", "revert-layer"}
)

// takesValue reports whether a property takes a value: one its entry in takenValues accepts, a
// CSS-wide keyword, or a value that holds a function (var(), calc(), ...), which is only known
// where it is used. A property without an entry takes any value.
func takesValue(prop, value string) bool {
	takes, known := takenValues[prop]
	if !known {
		return true
	}
	tokens := valueTokens(value)
	if slices.ContainsFunc(tokens, func(t css.Token) bool { return t.TokenType == css.FunctionToken }) {
		return true
	}
	if len(tokens) == 1 && identIn(tokens[0], cssWideKeywords...) {
		return true
	}
	return takes(tokens)
}

// valueTokens lexes a declaration's value into its tokens, without whitespace and comments, with
// each ident lower-case.
func valueTokens(value string) []css.Token {
	var out []css.Token
	l := css.NewLexer(parse.NewInputString(value))
	for {
		tt, data := l.Next()
		switch tt {
		case css.ErrorToken:
			return out
		case css.WhitespaceToken, css.CommentToken:
			continue
		case css.IdentToken:
			data = []byte(strings.ToLower(string(data)))
		default:
		}
		out = append(out, css.Token{TokenType: tt, Data: data})
	}
}

func identIn(t css.Token, words ...string) bool {
	return t.TokenType == css.IdentToken && slices.Contains(words, string(t.Data))
}

func allIdentsIn(tokens []css.Token, words []string) bool {
	return !slices.ContainsFunc(tokens, func(t css.Token) bool { return !identIn(t, words...) })
}

// lengthUnits are the units of a CSS length (the viewport ones also with an s, l or d prefix).
var lengthUnits = []string{
	"px", "pt", "pc", "cm", "mm", "q", "in", "em", "rem", "ex", "rex", "ch", "rch", "cap", "rcap",
	"ic", "ric", "lh", "rlh", "vw", "vh", "vi", "vb", "vmin", "vmax", "cqw", "cqh", "cqi", "cqb",
	"cqmin", "cqmax",
}

// isLength reports whether a token is a number with a length unit.
func isLength(t css.Token) bool {
	if t.TokenType != css.DimensionToken {
		return false
	}
	num, _ := parse.Dimension(t.Data)
	unit := strings.ToLower(string(t.Data[num:]))
	viewport := len(unit) > 2 && strings.ContainsRune("sld", rune(unit[0])) &&
		slices.Contains([]string{"vw", "vh", "vi", "vb", "vmin", "vmax"}, unit[1:])
	return slices.Contains(lengthUnits, unit) || viewport
}

// isZero reports whether a token is a unitless zero, the one number a length takes.
func isZero(t css.Token) bool {
	n, err := strconv.ParseFloat(string(t.Data), 64)
	return t.TokenType == css.NumberToken && err == nil && n == 0
}
