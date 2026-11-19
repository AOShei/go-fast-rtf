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
	// A lead and a trail byte are one character, not decoded.
	if got := decode(932, 0x82, 0xA0, 'a'); len(got) != 2 || got[0] != Unmapped || got[1] != 'a' {
		t.Errorf("932: %U", got)
	}
	// A lead byte with nothing after it.
	if got := decode(936, 0x81); len(got) != 1 || got[0] != Unmapped {
		t.Errorf("936: %U", got)
	}
	// A half-width katakana byte in 932 is not a lead byte.
	if got := decode(932, 0xB1, 'b'); len(got) != 2 || got[1] != 'b' {
		t.Errorf("932 single: %U", got)
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
