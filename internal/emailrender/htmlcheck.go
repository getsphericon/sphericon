package emailrender

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// optionalEnd are the elements whose end tag HTML lets an author leave out.
var optionalEnd = map[string]bool{
	"html": true, "head": true, "body": true, "p": true, "li": true, "dt": true, "dd": true,
	"tr": true, "td": true, "th": true, "thead": true, "tbody": true, "tfoot": true,
	"colgroup": true, "caption": true, "option": true, "optgroup": true,
}

// voidElements never have an end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// checkHTML is the parse check of ADR 0040. HTML5 parsing never fails by itself (the
// spec recovers from everything), so "does not parse" is a tokenizer error, a tag
// or comment left open at the end, an end tag that closes nothing, or an element
// that its parent's end tag leaves open.
func checkHTML(src string) error {
	z := html.NewTokenizer(strings.NewReader(src))
	var open []string
	for {
		switch z.Next() {
		case html.ErrorToken:
			err := z.Err()
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("html: %w", err)
			}
			if raw := z.Raw(); len(raw) > 0 && raw[0] == '<' {
				return errors.New("html: a tag or comment is not terminated")
			}
			if n := len(open); n > 0 {
				return fmt.Errorf("html: <%s> is never closed", open[n-1])
			}
			return nil
		case html.StartTagToken:
			name, _ := z.TagName()
			if !voidElements[string(name)] {
				open = append(open, string(name))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			var err error
			if open, err = closeTag(open, string(name)); err != nil {
				return err
			}
		}
	}
}

// closeTag pops the element name closes. Elements between it and the top of the stack
// are closed implicitly only when HTML allows their end tag to be omitted.
func closeTag(open []string, name string) ([]string, error) {
	for i, o := range slices.Backward(open) {
		if o != name {
			continue
		}
		for _, skipped := range open[i+1:] {
			if !optionalEnd[skipped] {
				return nil, fmt.Errorf("html: <%s> is never closed before </%s>", skipped, name)
			}
		}
		return open[:i], nil
	}
	return nil, fmt.Errorf("html: </%s> closes nothing", name)
}
