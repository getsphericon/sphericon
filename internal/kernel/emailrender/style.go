package emailrender

import (
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// parseStyle parses an inline style into property -> value (lower-case property), with
// tdewolff/parse's CSS parser, which keeps a value that holds a ";", a comma or parentheses
// (a url(), a quoted string, a function) whole. A malformed declaration is skipped and the
// parse goes on with the next one; the parser reports the end of the style as an error grammar
// without a parse error; a stray ")" or "]" still ends the parse, as the parser's recovery
// never closes it. A property declared twice keeps the declaration that applies: the
// later one, unless only the earlier one is !important. The value keeps its "!important".
func parseStyle(style string) map[string]string {
	out := map[string]string{}
	important := map[string]bool{}
	p := css.NewParser(parse.NewInputString(style), true)
	for {
		gt, _, data := p.Next()
		switch gt {
		case css.ErrorGrammar:
			if p.HasParseError() {
				continue
			}
			return out
		case css.DeclarationGrammar:
			prop, values := strings.ToLower(string(data)), p.Values()
			imp := isImportant(values)
			if important[prop] && !imp {
				continue
			}
			var value strings.Builder
			for _, t := range values {
				value.Write(t.Data)
			}
			out[prop], important[prop] = strings.TrimSpace(value.String()), imp
		default:
		}
	}
}

// isImportant reports whether a declaration's value ends in "!important" (the parser drops the
// whitespace around the "!").
func isImportant(values []css.Token) bool {
	n := len(values)
	return n >= 2 && values[n-2].TokenType == css.DelimToken && string(values[n-2].Data) == "!" &&
		values[n-1].TokenType == css.IdentToken && strings.EqualFold(string(values[n-1].Data), "important")
}
