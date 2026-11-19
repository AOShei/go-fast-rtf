package codepage

import "testing"

func decode(cp int, bs ...byte) []rune {
	var d Decoder
	var out []rune
	emit := func(r rune, ok bool) { out = append(out, r) }
	for _, b := range bs {
		d.Byte(cp, b, emit)
	}
	d.Flush(emit)
	return out
}

// TestTablesInjective checks that no two bytes of a table map to one
// character, which is true of every one of these code pages and catches
// a value written twice.
func TestTablesInjective(t *testing.T) {
	for _, cp := range []int{874, 1250, 1251, 1252, 1253, 1254, 1255, 1256, 1257, 1258, 10000} {
		seen := map[uint16]int{}
		for i, v := range table(cp) {
			if v == 0 {
				continue
			}
			if v < 0x80 {
				t.Errorf("cp%d: byte %#x maps into ASCII", cp, i+0x80)
			}
			if j, dup := seen[v]; dup {
				t.Errorf("cp%d: bytes %#x and %#x both map to U+%04X", cp, j+0x80, i+0x80, v)
			}
			seen[v] = i
		}
	}
}

func TestSpotValues(t *testing.T) {
	cases := []struct {
		cp   int
		b    byte
		want rune
	}{
		{1252, 0x80, 0x20AC}, {1252, 0x9F, 0x0178}, {1252, 0xE9, 0x00E9}, {1252, 0x81, Unmapped},
		{1250, 0xB9, 0x0105}, {1250, 0xE6, 0x0107}, {1250, 0x8C, 0x015A},
		{1251, 0xC0, 0x0410}, {1251, 0xFF, 0x044F}, {1251, 0xA8, 0x0401}, {1251, 0x88, 0x20AC},
		{1253, 0xC1, 0x0391}, {1253, 0xD2, Unmapped}, {1253, 0xFE, 0x03CE},
		{1254, 0xD0, 0x011E}, {1254, 0xFD, 0x0131},
		{1255, 0xE0, 0x05D0}, {1255, 0xFA, 0x05EA}, {1255, 0xA4, 0x20AA},
		{1256, 0xC7, 0x0627}, {1256, 0xFF, 0x06D2},
		{1257, 0xE0, 0x0105}, {1257, 0xFF, 0x02D9},
		{1258, 0xFE, 0x20AB}, {1258, 0xCC, 0x0300},
		{874, 0xA1, 0x0E01}, {874, 0xFB, 0x0E5B}, {874, 0xDB, Unmapped},
		{10000, 0x80, 0x00C4}, {10000, 0xDB, 0x20AC}, {10000, 0xF0, 0xF8FF},
		{Symbol, 0xB7, 0x2022},
		{437, 0x80, Unmapped},
	}
	for _, c := range cases {
		got := decode(c.cp, c.b)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("cp%d %#x: got %U, want %U", c.cp, c.b, got, c.want)
		}
	}
}

func TestDoubleByte(t *testing.T) {
	cases := []struct {
		cp   int
		in   []byte
		want []rune
	}{
		// Hiragana a, ideographic space, a kanji, then ASCII.
		{932, []byte{0x82, 0xA0, 0x81, 0x40, 0x93, 0xFA, 'a'}, []rune{0x3042, 0x3000, 0x65E5, 'a'}},
		// A half-width katakana byte in 932 is a character alone.
		{932, []byte{0xB1, 'b'}, []rune{0xFF71, 'b'}},
		// A trail byte in the ASCII range.
		{932, []byte{0x83, 0x41}, []rune{0x30A2}},
		{936, []byte{0xD6, 0xD0, 0xCE, 0xC4, 0xB0, 0xA1}, []rune{0x4E2D, 0x6587, 0x554A}},
		{949, []byte{0xB0, 0xA1, 0xC7, 0xD1, 0xA1, 0xA1}, []rune{0xAC00, 0xD55C, 0x3000}},
		{950, []byte{0xA4, 0x40, 0xA4, 0xA4, 0xB0, 0xA1}, []rune{0x4E00, 0x4E2D, 0x965B}},
		// A lead byte with nothing after it.
		{936, []byte{0x81}, []rune{Unmapped}},
		// A lead byte followed by a byte that cannot be a trail byte: the
		// lead is not decoded and the next byte is read afresh.
		{949, []byte{0xB0, '1'}, []rune{Unmapped, '1'}},
		// A pair the table does not map is one character.
		{950, []byte{0xFE, 0xFE, 'z'}, []rune{Unmapped, 'z'}},
	}
	for _, c := range cases {
		got := decode(c.cp, c.in...)
		if string(got) != string(c.want) {
			t.Errorf("cp%d % X: got %U, want %U", c.cp, c.in, got, c.want)
		}
	}
	// A lead byte left half read when the code page changes.
	var d Decoder
	var out []rune
	emit := func(r rune, ok bool) { out = append(out, r) }
	d.Byte(932, 0x82, emit)
	d.Byte(1252, 0xE9, emit)
	if string(out) != string([]rune{Unmapped, 0xE9}) {
		t.Errorf("code page change: %U", out)
	}
}

// TestTablesParsed checks the size of each double-byte table, so that a
// damaged or truncated table file is noticed.
func TestTablesParsed(t *testing.T) {
	for cp, min := range map[int]int{932: 7000, 936: 21000, 949: 17000, 950: 13000} {
		tb := dbcsTable(cp)
		n := 0
		for _, v := range tb.single {
			if v != 0 {
				n++
			}
		}
		for _, arr := range tb.lead {
			if arr != nil {
				for _, v := range arr {
					if v != 0 {
						n++
					}
				}
			}
		}
		if n < min {
			t.Errorf("cp%d: %d characters", cp, n)
		}
	}
}

func TestUTF8(t *testing.T) {
	if got := decode(65001, 0xE2, 0x82, 0xAC, 'x'); len(got) != 2 || got[0] != 0x20AC || got[1] != 'x' {
		t.Errorf("utf-8: %U", got)
	}
	if got := decode(65001, 0xE2, 'x'); len(got) != 2 || got[0] != Unmapped {
		t.Errorf("broken utf-8: %U", got)
	}
}

func TestForCharset(t *testing.T) {
	for cs, cp := range map[int]int{0: 1252, 1: 0, 2: Symbol, 128: 932, 134: 936, 204: 1251, 238: 1250, 222: 874, 77: 10000} {
		if got := ForCharset(cs); got != cp {
			t.Errorf("charset %d: %d, want %d", cs, got, cp)
		}
	}
}
