package smsrender

import "fmt"

// Encoding is the alphabet a text goes out in, which sets how many characters a part holds.
type Encoding string

const (
	// GSM7 is the GSM 03.38 default alphabet: 160 characters in one part, 153 per part when the
	// text is split.
	GSM7 Encoding = "GSM-7"
	// UCS2 is any text with a character outside GSM-7 (any Cyrillic, any emoji): 70 characters in
	// one part, 67 per part when the text is split.
	UCS2 Encoding = "UCS-2"
)

// Size is a text as an SMS counts it: its encoding, its characters in that encoding (a GSM-7
// extension character is two, a UCS-2 character beyond the BMP is two UTF-16 code units) and the
// parts it is sent in. src/utils/smsParts.ts counts the same way, and both agree on
// testdata/parts.json.
type Size struct {
	Encoding   Encoding
	Characters int
	Parts      int
}

// gsm7Basic is the default alphabet; gsm7Extension the characters sent as an escape and a second
// septet.
const (
	gsm7Basic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
		"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
	gsm7Extension = "\f^{}\\[~]|€"
)

var gsm7Width = func() map[rune]int {
	widths := map[rune]int{}
	for _, r := range gsm7Basic {
		widths[r] = 1
	}
	for _, r := range gsm7Extension {
		widths[r] = 2
	}
	return widths
}()

// Measure counts text as written: merge tags count as their own characters, since what they
// render to is the Contact's.
func Measure(text string) Size {
	septets, gsm := 0, true
	for _, r := range text {
		w, ok := gsm7Width[r]
		if !ok {
			gsm = false
			break
		}
		septets += w
	}
	if gsm {
		return Size{Encoding: GSM7, Characters: septets, Parts: parts(septets, 160, 153)}
	}
	units := 0
	for _, r := range text {
		units++
		if r > 0xFFFF {
			units++
		}
	}
	return Size{Encoding: UCS2, Characters: units, Parts: parts(units, 70, 67)}
}

// parts is how many parts n characters take: one up to single, else n split into parts of each.
func parts(n, single, each int) int {
	if n <= single {
		return min(n, 1)
	}
	return (n + each - 1) / each
}

// TooManyPartsError is a text that, as written, takes more parts than the provider sends.
type TooManyPartsError struct {
	Size  Size
	Limit int
}

func (e *TooManyPartsError) Error() string {
	return fmt.Sprintf("the text is %d characters in %s, %d segments; it may take %d segments at most",
		e.Size.Characters, e.Size.Encoding, e.Size.Parts, e.Limit)
}

// CheckParts refuses a text that, as written, takes more than limit parts, the provider's part
// limit. The error is a *TooManyPartsError.
func CheckParts(text string, limit int) error {
	if size := Measure(text); size.Parts > limit {
		return &TooManyPartsError{Size: size, Limit: limit}
	}
	return nil
}
