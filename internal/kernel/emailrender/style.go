package emailrender

import (
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// parseStyle parses an inline style into property -> value (lower-case property), with
// tdewolff/parse's CSS parser, which keeps a value that holds a ";", a comma or parentheses
// (a url(), a quoted string, a function) whole. A malformed declaration is skipped and the
// parse goes on with the next one. A property declared twice keeps the declaration that
// applies: the later one, unless only the earlier one is !important. The value is returned
// without its "!important".
func parseStyle(style string) map[string]string {
	out := map[string]string{}
	important := map[string]bool{}
	for _, decl := range declarations(style) {
		p := css.NewParser(parse.NewInputString(decl), true)
		for {
			gt, _, data := p.Next()
			if gt == css.ErrorGrammar && !p.HasParseError() {
				break // the end of the declaration; the parser reports it as an error grammar
			}
			if gt != css.DeclarationGrammar {
				continue
			}
			prop := strings.ToLower(string(data))
			values, isImportant := withoutImportant(p.Values())
			if important[prop] && !isImportant {
				continue
			}
			var value strings.Builder
			for _, t := range values {
				value.Write(t.Data)
			}
			out[prop], important[prop] = strings.TrimSpace(value.String()), isImportant
		}
	}
	return out
}

// declarations splits an inline style at each ";" outside parentheses, brackets and braces,
// with the parser's own lexer, so a ";" inside a url() or a quoted string stays in its value.
// The parser could split it alone, but its recovery from a malformed declaration counts a stray
// ")" or "]" as a close below the top level and reads the rest of the style as that one
// declaration; here a stray closer is ignored, and only its own declaration is lost.
func declarations(style string) []string {
	var out []string
	var decl strings.Builder
	depth := 0
	l := css.NewLexer(parse.NewInputString(style))
	for {
		tt, data := l.Next()
		switch tt {
		case css.ErrorToken:
			return append(out, decl.String())
		case css.SemicolonToken:
			if depth == 0 {
				out = append(out, decl.String())
				decl.Reset()
				continue
			}
		case css.FunctionToken, css.LeftParenthesisToken, css.LeftBracketToken, css.LeftBraceToken:
			depth++
		case css.RightParenthesisToken, css.RightBracketToken, css.RightBraceToken:
			depth = max(depth-1, 0)
		default:
		}
		decl.Write(data)
	}
}

// withoutImportant drops the "!important" a declaration's value ends in (the parser has already
// dropped the whitespace around the "!") and reports whether it was there.
func withoutImportant(values []css.Token) ([]css.Token, bool) {
	n := len(values)
	marked := n >= 2 && values[n-2].TokenType == css.DelimToken && string(values[n-2].Data) == "!" &&
		values[n-1].TokenType == css.IdentToken && strings.EqualFold(string(values[n-1].Data), "important")
	if !marked {
		return values, false
	}
	return values[:n-2], true
}
