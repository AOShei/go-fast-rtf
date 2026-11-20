// Package codepage turns the bytes of RTF text (\'hh escapes and 8-bit
// text) into Unicode, for the code pages RTF names with \ansicpgN, \cpgN
// and \fcharsetN.
//
// The single-byte Windows code pages 874 and 1250 to 1258 and the Mac Roman
// page (10000) are decoded from tables, and so are the double-byte code
// pages 932, 936, 949 and 950, a character at a time, lead and trail byte
// together. A character a table does not map becomes U+FFFD. Any other
// code page is not decoded, byte by byte above 0x7F.
package codepage

// Symbol is the pseudo code page of a font with \fcharset2 (Symbol). Its
// bytes are the font's glyph numbers; the few that stand for bullets are
// mapped, the rest are not decoded.
const Symbol = -2

// Known reports whether bytes in code page cp are decoded.
func Known(cp int) bool { return table(cp) != nil || cp == 65001 || DoubleByte(cp) }

// DoubleByte reports whether cp is one of the double-byte code pages this
// package decodes.
func DoubleByte(cp int) bool {
	switch cp {
	case 932, 936, 949, 950:
		return true
	}
	return false
}

func table(cp int) *[128]uint16 {
	switch cp {
	case 874:
		return &cp874
	case 1250:
		return &cp1250
	case 1251:
		return &cp1251
	case 1252:
		return &cp1252
	case 1253:
		return &cp1253
	case 1254:
		return &cp1254
	case 1255:
		return &cp1255
	case 1256:
		return &cp1256
	case 1257:
		return &cp1257
	case 1258:
		return &cp1258
	case 10000:
		return &macRoman
	}
	return nil
}

// isLead reports whether b is a lead byte of double-byte code page cp.
func isLead(cp int, b byte) bool {
	switch cp {
	case 932:
		return b >= 0x81 && b <= 0x9F || b >= 0xE0 && b <= 0xFC
	case 936, 949, 950:
		return b >= 0x81 && b <= 0xFE
	}
	return false
}

// ForCharset returns the code page of a font's \fcharsetN, or 0 when the
// charset says nothing more than the document's code page.
func ForCharset(charset int) int {
	switch charset {
	case 0:
		return 1252
	case 2:
		return Symbol
	case 77:
		return 10000
	case 128:
		return 932
	case 129:
		return 949
	case 130:
		return 1361
	case 134:
		return 936
	case 136:
		return 950
	case 161:
		return 1253
	case 162:
		return 1254
	case 163:
		return 1258
	case 177:
		return 1255
	case 178:
		return 1256
	case 186:
		return 1257
	case 204:
		return 1251
	case 222:
		return 874
	case 238:
		return 1250
	case 254:
		return 437
	case 255:
		return 850
	}
	return 0
}

// Decoder decodes bytes one at a time, holding the lead byte of a
// double-byte character until its trail byte comes.
type Decoder struct {
	lead     bool
	leadCP   int
	leadByte byte
	utf8     []byte
	pending  int // bytes of a UTF-8 sequence still expected
}

// Unmapped is returned for a byte or character that is not decoded.
const Unmapped = 0xFFFD

// Byte decodes byte b of code page cp. It returns the characters it
// completes (none while a double-byte character is half read), and for
// each character that was not decoded, a U+FFFD and ok false.
func (d *Decoder) Byte(cp int, b byte, emit func(r rune, ok bool)) {
	if d.lead {
		d.lead = false
		if d.leadCP == cp && b >= trailFirst {
			// A lead and a trail byte are one character, mapped or not.
			if t := dbcsTable(cp); t != nil {
				if r := t.pair(d.leadByte, b); r != 0 {
					emit(r, true)
					return
				}
			}
			if r := userDefined(cp, d.leadByte, b); r != 0 {
				emit(r, true)
				return
			}
			emit(Unmapped, false)
			return
		}
		// A lead byte not followed by a trail byte is not a character;
		// what follows it is read afresh.
		emit(Unmapped, false)
	}
	if cp == 65001 {
		d.utf8Byte(b, emit)
		return
	}
	d.flushUTF8(emit)
	if b < 0x80 {
		emit(rune(b), true)
		return
	}
	if t := table(cp); t != nil {
		if r := t[b-0x80]; r != 0 {
			emit(rune(r), true)
			return
		}
		emit(Unmapped, false)
		return
	}
	if cp == Symbol {
		switch b {
		case 0xB7:
			emit(0x2022, true) // bullet
		default:
			emit(Unmapped, false)
		}
		return
	}
	if isLead(cp, b) {
		d.lead, d.leadCP, d.leadByte = true, cp, b
		return
	}
	if t := dbcsTable(cp); t != nil {
		if r := t.single[b-0x80]; r != 0 {
			emit(rune(r), true)
			return
		}
	}
	if cp == 932 && b == 0x80 {
		// Windows and Python's cp932 read 0x80 alone as U+0080.
		emit(0x80, true)
		return
	}
	emit(Unmapped, false)
}

// userDefined returns the Private Use Area character Windows gives a
// lead and trail byte in a user-defined area of code page 932 or 936,
// which the tables leave out, or 0. Code pages 949 and 950 are not
// mapped here.
func userDefined(cp int, lead, trail byte) rune {
	switch cp {
	case 932:
		// Lead bytes 0xF0-0xF9, trail bytes 0x40-0x7E and 0x80-0xFC
		// (188 per lead byte), to U+E000-U+E757.
		if lead < 0xF0 || lead > 0xF9 || trail < 0x40 || trail == 0x7F || trail > 0xFC {
			return 0
		}
		off := rune(trail - 0x40)
		if trail > 0x7F {
			off--
		}
		return 0xE000 + rune(lead-0xF0)*188 + off
	case 936:
		switch {
		case lead >= 0xAA && lead <= 0xAF && trail >= 0xA1 && trail <= 0xFE:
			// 0xAAA1-0xAFFE, 94 per lead byte, to U+E000-U+E233.
			return 0xE000 + rune(lead-0xAA)*94 + rune(trail-0xA1)
		case lead >= 0xF8 && lead <= 0xFE && trail >= 0xA1 && trail <= 0xFE:
			// 0xF8A1-0xFEFE, 94 per lead byte, to U+E234-U+E4C5.
			return 0xE234 + rune(lead-0xF8)*94 + rune(trail-0xA1)
		case lead >= 0xA1 && lead <= 0xA7 && trail >= 0x40 && trail <= 0xA0 && trail != 0x7F:
			// 0xA140-0xA7A0, trail bytes 0x40-0x7E and 0x80-0xA0 (96 per
			// lead byte), to U+E4C6-U+E765.
			off := rune(trail - 0x40)
			if trail > 0x7F {
				off--
			}
			return 0xE4C6 + rune(lead-0xA1)*96 + off
		}
	}
	return 0
}

// Flush ends a run of bytes: a lead byte with no trail byte, or an
// unfinished UTF-8 sequence, is not decoded.
func (d *Decoder) Flush(emit func(r rune, ok bool)) {
	if d.lead {
		d.lead = false
		emit(Unmapped, false)
	}
	d.flushUTF8(emit)
}

func (d *Decoder) utf8Byte(b byte, emit func(r rune, ok bool)) {
	if d.pending > 0 && b&0xC0 == 0x80 {
		d.utf8 = append(d.utf8, b)
		d.pending--
		if d.pending == 0 {
			r := decodeUTF8(d.utf8)
			d.utf8 = d.utf8[:0]
			emit(r, r != Unmapped)
		}
		return
	}
	d.flushUTF8(emit)
	switch {
	case b < 0x80:
		emit(rune(b), true)
	case b&0xE0 == 0xC0:
		d.utf8, d.pending = append(d.utf8[:0], b), 1
	case b&0xF0 == 0xE0:
		d.utf8, d.pending = append(d.utf8[:0], b), 2
	case b&0xF8 == 0xF0:
		d.utf8, d.pending = append(d.utf8[:0], b), 3
	default:
		emit(Unmapped, false)
	}
}

func (d *Decoder) flushUTF8(emit func(r rune, ok bool)) {
	if d.pending > 0 {
		d.pending = 0
		d.utf8 = d.utf8[:0]
		emit(Unmapped, false)
	}
}

func decodeUTF8(b []byte) rune {
	var r rune
	switch len(b) {
	case 2:
		r = rune(b[0]&0x1F)<<6 | rune(b[1]&0x3F)
		if r < 0x80 {
			return Unmapped
		}
	case 3:
		r = rune(b[0]&0x0F)<<12 | rune(b[1]&0x3F)<<6 | rune(b[2]&0x3F)
		if r < 0x800 || r >= 0xD800 && r <= 0xDFFF {
			return Unmapped
		}
	case 4:
		r = rune(b[0]&0x07)<<18 | rune(b[1]&0x3F)<<12 | rune(b[2]&0x3F)<<6 | rune(b[3]&0x3F)
		if r < 0x10000 || r > 0x10FFFF {
			return Unmapped
		}
	default:
		return Unmapped
	}
	return r
}
