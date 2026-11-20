package codepage

import "testing"

// TestUserDefinedAreas checks the first and last code point of each
// user-defined area of code pages 932 and 936, and a byte pair just
// outside each, as Windows decodes them.
func TestUserDefinedAreas(t *testing.T) {
	const bad = Unmapped
	cases := []struct {
		cp    int
		bytes []byte
		want  []rune
	}{
		// Code page 932: 0x80 alone, and lead bytes 0xF0-0xF9.
		{932, []byte{0x80}, []rune{0x80}},
		{932, []byte{0xF0, 0x40}, []rune{0xE000}},
		{932, []byte{0xF0, 0x7E}, []rune{0xE03E}},
		{932, []byte{0xF0, 0x80}, []rune{0xE03F}},
		{932, []byte{0xF0, 0xFC}, []rune{0xE0BB}},
		{932, []byte{0xF1, 0x40}, []rune{0xE0BC}},
		{932, []byte{0xF9, 0xFC}, []rune{0xE757}},
		{932, []byte{0xF0, 0x7F}, []rune{bad}},
		{932, []byte{0xF9, 0xFD}, []rune{bad}},
		{932, []byte{0xEF, 0x40}, []rune{bad}},

		// Code page 936: 0xAAA1-0xAFFE.
		{936, []byte{0xAA, 0xA1}, []rune{0xE000}},
		{936, []byte{0xAB, 0xA1}, []rune{0xE05E}},
		{936, []byte{0xAF, 0xFE}, []rune{0xE233}},
		{936, []byte{0xAA, 0xA0}, []rune{0x7371}}, // GBK, not user-defined
		{936, []byte{0xAF, 0xFF}, []rune{bad}},
		// 0xF8A1-0xFEFE.
		{936, []byte{0xF8, 0xA1}, []rune{0xE234}},
		{936, []byte{0xFE, 0xFE}, []rune{0xE4C5}},
		{936, []byte{0xF8, 0xA0}, []rune{0x9D42}}, // GBK, not user-defined
		// 0xA140-0xA7A0, trail bytes 0x40-0x7E and 0x80-0xA0.
		{936, []byte{0xA1, 0x40}, []rune{0xE4C6}},
		{936, []byte{0xA1, 0x7E}, []rune{0xE504}},
		{936, []byte{0xA1, 0x80}, []rune{0xE505}},
		{936, []byte{0xA1, 0xA0}, []rune{0xE525}},
		{936, []byte{0xA2, 0x40}, []rune{0xE526}},
		{936, []byte{0xA7, 0xA0}, []rune{0xE765}},
		{936, []byte{0xA1, 0x7F}, []rune{bad}},
		{936, []byte{0xA8, 0x40}, []rune{0x02CA}}, // GBK, not user-defined
		{936, []byte{0xA1, 0xA1}, []rune{0x3000}}, // GB 2312 just after

		// Code pages 949 and 950 are unchanged.
		{949, []byte{0xC9, 0xA1}, []rune{bad}},
		{950, []byte{0x80}, []rune{bad}},
	}
	for _, c := range cases {
		got := decode(c.cp, c.bytes...)
		if len(got) != len(c.want) {
			t.Errorf("cp%d % X: got %U, want %U", c.cp, c.bytes, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("cp%d % X: got %U, want %U", c.cp, c.bytes, got, c.want)
				break
			}
		}
	}
}
