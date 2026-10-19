package emaildoc

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// The editor's own loader is lenient: it drops unknown attributes, accepts a heading of level 9, an
// empty button and a link without an href, and replaces a document it cannot read with an empty
// paragraph. These rules are ours, over the document's JSON, and they refuse instead, naming the
// place and what to write. A reason is the whole message an agent gets, so it says what is wrong,
// where, and what is allowed.

var (
	alignments = []string{"left", "center", "right"}
	// Schemes a link or image may use. An absolute URL, a {{ variable }} (Liquid fills it in at send
	// time) or, for a link, an in-page "#anchor". A relative path means nothing in a mail client.
	urlSchemes = []string{"http", "https", "mailto", "tel"}

	schemePrefix  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	sizeValue     = regexp.MustCompile(`^(auto|[0-9]+(\.[0-9]+)?(px|%)?)$`)
	liquidValue   = regexp.MustCompile(`^\{\{[^{}]+\}\}`)
	liquidTags    = regexp.MustCompile(`\{\{[^{}]*\}\}|\{%[^{}]*%\}`)
	unsafeStyle   = regexp.MustCompile(`(?i)javascript:|vbscript:|expression\s*\(|behavior\s*:|-moz-binding|@import|<|\\`)
	orderedTypes  = []string{"1", "a", "A", "i", "I"}
	nodeProps     = []string{"type", "attrs", "content", "marks", "text"}
	markProps     = []string{"type", "attrs"}
	controlsInURL = regexp.MustCompile(`[\x00-\x20\x7f]`)
)

// checkRules returns an *InvalidError for the first thing wrong with the document, or nil.
func checkRules(root any) error {
	object, ok := root.(map[string]any)
	if !ok {
		return invalidf("The document must be a JSON object: {\"type\":\"doc\",\"content\":[...]}.")
	}
	if object["type"] != "doc" {
		return invalidf("The document's root must have type \"doc\", got %s.", show(object["type"]))
	}
	content, ok := object["content"].([]any)
	if !ok || len(content) == 0 {
		return invalidf("The document has no content: \"content\" must be a non-empty array of nodes.")
	}
	return checkChildren(object, "doc")
}

func checkChildren(parent map[string]any, path string) error {
	children, ok := parent["content"].([]any)
	if !ok {
		return nil
	}
	for i, child := range children {
		if err := checkNode(child, fmt.Sprintf("%s.content[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func checkNode(value any, path string) error {
	node, ok := value.(map[string]any)
	if !ok {
		return invalidf("%s must be an object like {\"type\":\"paragraph\",\"content\":[...]}, got %s.", path, show(value))
	}
	name, ok := node["type"].(string)
	if !ok {
		return invalidf("%s has no \"type\": every node needs one of %s.", path, quoted(nodeNames()))
	}
	where := fmt.Sprintf("%s (%s)", path, name)
	schema := EditorSchema()
	def, known := schema.Nodes[name]
	if !known || name == "doc" {
		return invalidf("%s: unknown node type %q. Use one of %s.", where, name, quoted(nodeNames()))
	}
	for _, prop := range slices.Sorted(maps.Keys(node)) {
		if !slices.Contains(nodeProps, prop) {
			return invalidf("%s: unknown property %q on a node. A node has only %s; put %s inside \"attrs\".", where, prop, quoted(nodeProps), show(prop))
		}
	}
	if name == "text" {
		if err := checkText(node, where); err != nil {
			return err
		}
	} else if _, has := node["text"]; has {
		return invalidf("%s: only a text node has \"text\". Wrap the words in {\"type\":\"text\",\"text\":\"...\"} inside \"content\".", where)
	}
	attrs, err := attrsOf(node["attrs"], where)
	if err != nil {
		return err
	}
	if err := checkAttrNames(attrs, def, where, "node"); err != nil {
		return err
	}
	if err := checkNodeAttrs(name, attrs, node, where); err != nil {
		return err
	}
	if err := checkMarks(node["marks"], where); err != nil {
		return err
	}
	return checkChildren(node, path)
}

func checkText(node map[string]any, where string) error {
	text, ok := node["text"].(string)
	if !ok || text == "" {
		return invalidf("%s: a text node needs a non-empty \"text\" string; leave an empty paragraph without content instead of an empty text node.", where)
	}
	if _, has := node["content"]; has {
		return invalidf("%s: a text node has no \"content\".", where)
	}
	return nil
}

func attrsOf(value any, where string) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	attrs, ok := value.(map[string]any)
	if !ok {
		return nil, invalidf("%s: \"attrs\" must be an object, got %s.", where, show(value))
	}
	return attrs, nil
}

func checkAttrNames(attrs map[string]any, def SchemaType, where, kind string) error {
	for _, attr := range slices.Sorted(maps.Keys(attrs)) {
		if _, ok := def.Attrs[attr]; !ok {
			accepted := "none"
			if len(def.Attrs) > 0 {
				accepted = quoted(slices.Sorted(maps.Keys(def.Attrs)))
			}
			return invalidf("%s: this %s has no attribute %q (it would be dropped without a word). Its attributes are: %s.", where, kind, attr, accepted)
		}
	}
	return nil
}

func checkNodeAttrs(name string, attrs map[string]any, node map[string]any, where string) error {
	for _, attr := range []string{"alignment", "align"} {
		if v, ok := attrs[attr]; ok && v != nil {
			if s, isString := v.(string); !isString || !slices.Contains(alignments, s) {
				return invalidf("%s: attribute %q must be one of %s, got %s.", where, attr, quoted(alignments), show(v))
			}
		}
	}
	if v, ok := attrs["style"]; ok {
		if err := checkStyle(v, where); err != nil {
			return err
		}
	}
	switch name {
	case "heading":
		if v, ok := attrs["level"]; ok && !wholeBetween(v, 1, 6) {
			return invalidf("%s: attribute \"level\" must be a whole number from 1 to 6, got %s.", where, show(v))
		}
	case "button":
		return checkButton(attrs, node, where)
	case "image":
		return checkImage(attrs, where)
	case "orderedList":
		if v, ok := attrs["start"]; ok && v != nil && !wholeBetween(v, 1, 1<<30) {
			return invalidf("%s: attribute \"start\" must be a whole number of 1 or more, got %s.", where, show(v))
		}
		if v, ok := attrs["type"]; ok && v != nil {
			if s, isString := v.(string); !isString || !slices.Contains(orderedTypes, s) {
				return invalidf("%s: attribute \"type\" must be one of %s, got %s.", where, quoted(orderedTypes), show(v))
			}
		}
	}
	return nil
}

func checkButton(attrs map[string]any, node map[string]any, where string) error {
	href, _ := attrs["href"].(string)
	if href == "" || href == "#" {
		return invalidf("%s: a button needs attrs.href, the address it opens, like {\"type\":\"button\",\"attrs\":{\"href\":\"https://example.com\"},\"content\":[{\"type\":\"text\",\"text\":\"Label\"}]}. Its label is text in \"content\", not an attribute.", where)
	}
	if err := checkURL(href, "href", where, false); err != nil {
		return err
	}
	if content, _ := node["content"].([]any); len(content) == 0 {
		return invalidf("%s: a button needs a label: text nodes in \"content\".", where)
	}
	return nil
}

func checkImage(attrs map[string]any, where string) error {
	src, _ := attrs["src"].(string)
	if src == "" {
		return invalidf("%s: an image needs attrs.src, its absolute https URL.", where)
	}
	if err := checkURL(src, "src", where, false); err != nil {
		return err
	}
	if alt, ok := attrs["alt"]; ok && alt != nil {
		if _, isString := alt.(string); !isString {
			return invalidf("%s: attribute \"alt\" must be text, got %s.", where, show(alt))
		}
	}
	if href, ok := attrs["href"]; ok && href != nil {
		s, isString := href.(string)
		if !isString {
			return invalidf("%s: attribute \"href\" must be a URL, got %s.", where, show(href))
		}
		if err := checkURL(s, "href", where, false); err != nil {
			return err
		}
	}
	for _, attr := range []string{"width", "height"} {
		if v, ok := attrs[attr]; ok && v != nil && !validSize(v) {
			return invalidf("%s: attribute %q must be \"auto\", a number of pixels or a size like \"50%%\", got %s.", where, attr, show(v))
		}
	}
	return nil
}

func checkMarks(value any, where string) error {
	if value == nil {
		return nil
	}
	marks, ok := value.([]any)
	if !ok {
		return invalidf("%s: \"marks\" must be an array of objects like {\"type\":\"bold\"}, got %s.", where, show(value))
	}
	schema := EditorSchema()
	for i, v := range marks {
		here := fmt.Sprintf("%s.marks[%d]", where, i)
		mark, ok := v.(map[string]any)
		if !ok {
			return invalidf("%s must be an object like {\"type\":\"bold\"}, got %s.", here, show(v))
		}
		name, ok := mark["type"].(string)
		if !ok {
			return invalidf("%s has no \"type\": every mark needs one of %s.", here, quoted(slices.Sorted(maps.Keys(schema.Marks))))
		}
		def, known := schema.Marks[name]
		if !known {
			return invalidf("%s: unknown mark type %q. Use one of %s.", here, name, quoted(slices.Sorted(maps.Keys(schema.Marks))))
		}
		for _, prop := range slices.Sorted(maps.Keys(mark)) {
			if !slices.Contains(markProps, prop) {
				return invalidf("%s: unknown property %q on a mark. A mark has only %s.", here, prop, quoted(markProps))
			}
		}
		attrs, err := attrsOf(mark["attrs"], here)
		if err != nil {
			return err
		}
		if err := checkAttrNames(attrs, def, here, "mark"); err != nil {
			return err
		}
		if v, ok := attrs["style"]; ok {
			if err := checkStyle(v, here); err != nil {
				return err
			}
		}
		if name == "link" {
			href, _ := attrs["href"].(string)
			if href == "" {
				return invalidf("%s: a link needs attrs.href, the address it opens, like {\"type\":\"link\",\"attrs\":{\"href\":\"https://example.com\"}}.", here)
			}
			if err := checkURL(href, "href", here, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkStyle(value any, where string) error {
	style, ok := value.(string)
	if !ok {
		return invalidf("%s: attribute \"style\" must be a CSS declaration string like \"color:#333333\", got %s.", where, show(value))
	}
	if unsafeStyle.MatchString(style) {
		return invalidf("%s: attribute \"style\" holds something an email cannot safely carry (a script URL, expression(), an @import, a backslash or a tag). Use plain declarations like \"color:#333333\".", where)
	}
	return nil
}

// checkURL holds a link or image address to an absolute URL of an allowed scheme, a {{ variable }},
// or (for links only) an in-page anchor.
func checkURL(value, attr, where string, anchorOK bool) error {
	// Liquid tags may hold spaces ({{ token | url_encode }}); anything else may not.
	plain := liquidTags.ReplaceAllString(value, "x")
	switch {
	case controlsInURL.MatchString(plain):
		return invalidf("%s: %s %s has a space or control character in it; write the URL without any.", where, attr, show(value))
	case liquidValue.MatchString(value):
		return nil
	case anchorOK && strings.HasPrefix(value, "#") && len(value) > 1:
		return nil
	}
	scheme := strings.TrimSuffix(schemePrefix.FindString(value), ":")
	if scheme == "" || !slices.Contains(urlSchemes, strings.ToLower(scheme)) {
		return invalidf("%s: %s %s must be an absolute URL starting with %s, or a {{ variable }}.", where, attr, show(value), quoted(prefixed(urlSchemes)))
	}
	return nil
}

func prefixed(schemes []string) []string {
	out := make([]string, len(schemes))
	for i, s := range schemes {
		out[i] = s + ":"
	}
	return out
}

func wholeBetween(value any, lo, hi int64) bool {
	n, ok := value.(json.Number)
	if !ok {
		return false
	}
	i, err := n.Int64()
	return err == nil && i >= lo && i <= hi
}

func validSize(value any) bool {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		return err == nil && f > 0
	case string:
		return sizeValue.MatchString(v)
	default:
		return false
	}
}

func nodeNames() []string {
	names := slices.Sorted(maps.Keys(EditorSchema().Nodes))
	return slices.DeleteFunc(names, func(n string) bool { return n == "doc" })
}

func quoted(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(out, ", ")
}

// show prints a JSON value for a reason, cut short so a huge value cannot flood it.
func show(value any) string {
	text, err := json.Marshal(value)
	if err != nil {
		return "a value that is not JSON"
	}
	const limit = 60
	if len(text) > limit {
		return string(text[:limit]) + "..."
	}
	return string(text)
}

func invalidf(format string, args ...any) error {
	return &InvalidError{Reason: fmt.Sprintf(format, args...)}
}
