// Package attachment is how a download names the file a browser saves: the one Content-Disposition
// of every surface's file responses.
package attachment

import "mime"

// ContentDisposition is the Content-Disposition of a response a browser saves as the file name,
// quoted or encoded as the name needs (mime.FormatMediaType).
func ContentDisposition(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}
