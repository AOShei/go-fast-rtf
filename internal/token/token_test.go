package token

import (
	"strings"
	"testing"
)

func all(src string, keep int64) []Token {
	l := New(strings.NewReader(src))
	l.KeepBin = keep
	var out []Token
	for {
		t := l.Next()
		if t.Kind == EOF {
			return out
		}
		t.Data = append([]byte(nil), t.Data...)
		out = append(out, t)
	}
}

func TestTokens(t *testing.T) {
	toks := all("{\\rtf1\\b0 Hi\\'e9\\u-3913?\\\\\\{\r\n\\par\\*\\x-y\\bin3 abcZ}", 0)
	want := []struct {
		k     Kind
		name  string
		param int32
		data  string
	}{
		{Open, "", 0, ""}, {Word, "rtf", 1, ""}, {Word, "b", 0, ""}, {Text, "", 0, "Hi"},
		{Hex, "", 0, ""}, {Word, "u", -3913, ""}, {Text, "", 0, "?"}, {Symbol, "", 0, ""},
		{Symbol, "", 0, ""}, {Word, "par", 0, ""}, {Symbol, "", 0, ""}, {Word, "x", 0, ""},
		{Text, "", 0, "-y"}, {Bin, "", 0, ""}, {Text, "", 0, "Z"}, {Close, "", 0, ""},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	for i, w := range want {
		g := toks[i]
		if g.Kind != w.k || g.Name != w.name || g.Param != w.param || (w.data != "" && string(g.Data) != w.data) {
			t.Errorf("token %d: %+v, want %+v", i, g, w)
		}
	}
	if toks[4].Sym != 0xE9 || toks[13].BinLen != 3 || toks[13].Data != nil {
		t.Errorf("hex %#x bin %+v", toks[4].Sym, toks[13])
	}
}

func TestBinKeep(t *testing.T) {
	toks := all("\\bin5 abcdeX", 3)
	if len(toks) != 2 || string(toks[0].Data) != "abc" || toks[0].BinLen != 5 || string(toks[1].Data) != "X" {
		t.Fatalf("%+v", toks)
	}
	// A length beyond the end of the input reads to the end.
	toks = all("\\bin999999999999 ab", 100)
	if len(toks) != 1 || toks[0].BinLen != 2 {
		t.Fatalf("%+v", toks)
	}
}

func TestLongWord(t *testing.T) {
	toks := all("\\"+strings.Repeat("a", 100)+"123456789012345 x", 0)
	if len(toks) != 2 || len(toks[0].Name) != maxName || toks[0].Param != 1<<31-1 {
		t.Fatalf("%+v", toks)
	}
}

func FuzzLexer(f *testing.F) {
	f.Add([]byte("{\\rtf1\\ansi{\\fonttbl\\f0 x;}\\'e9\\u-10\\bin2 ab}"))
	f.Add([]byte("\\'4"))
	f.Add([]byte("\\bin-5 x"))
	f.Fuzz(func(t *testing.T, b []byte) {
		l := New(strings.NewReader(string(b)))
		l.KeepBin = 16
		for i := 0; i <= len(b)+1; i++ {
			tok := l.Next()
			if tok.Kind == EOF {
				return
			}
			if len(tok.Data) > maxText {
				t.Fatalf("token of %d bytes", len(tok.Data))
			}
		}
		if l.Next().Kind != EOF {
			t.Fatalf("more tokens than bytes")
		}
	})
}
