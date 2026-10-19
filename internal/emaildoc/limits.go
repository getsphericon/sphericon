package emaildoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// parseWithin reads the document's JSON under the size, depth and node-count caps. It is the only
// thing that runs before a runtime does, so it never recurses deeper than the caps allow: the
// depth of the JSON itself is measured token by token before anything is decoded.
func parseWithin(doc []byte, l Limits) (any, error) {
	if len(doc) > l.MaxBytes {
		return nil, fmt.Errorf("%w: it is %d bytes and the limit is %d", ErrTooLarge, len(doc), l.MaxBytes)
	}
	if err := scanDepth(doc, 2*l.MaxDepth+16); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, &InvalidError{Reason: "The document is not valid JSON: " + err.Error()}
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, &InvalidError{Reason: "The document is not valid JSON: there is more after the document's closing brace."}
	}
	count := 0
	if err := countNodes(root, 1, l, &count); err != nil {
		return nil, err
	}
	return root, nil
}

// scanDepth fails when the JSON nests deeper than max, or is not JSON at all.
func scanDepth(doc []byte, limit int) error {
	decoder := json.NewDecoder(bytes.NewReader(doc))
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return &InvalidError{Reason: "The document is not valid JSON: " + err.Error()}
		}
		if delim, ok := token.(json.Delim); ok {
			switch delim {
			case '{', '[':
				depth++
				if depth > limit {
					return fmt.Errorf("%w: it nests more than %d levels of JSON", ErrTooDeep, limit)
				}
			case '}', ']':
				depth--
			}
		}
	}
}

// countNodes walks the document's node tree (objects in `content` arrays), holding it to the node
// and depth caps.
func countNodes(node any, depth int, l Limits, count *int) error {
	if depth > l.MaxDepth {
		return fmt.Errorf("%w: nodes nest more than %d levels and the limit is %d", ErrTooDeep, depth-1, l.MaxDepth)
	}
	*count++
	if *count > l.MaxNodes {
		return fmt.Errorf("%w: more than %d, the limit", ErrTooManyNodes, l.MaxNodes)
	}
	object, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	children, ok := object["content"].([]any)
	if !ok {
		return nil
	}
	for _, child := range children {
		if err := countNodes(child, depth+1, l, count); err != nil {
			return err
		}
	}
	return nil
}
