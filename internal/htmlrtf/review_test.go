package htmlrtf

import (
	"context"
	"strings"
	"testing"
)

const uw = "\\" + "u"

func TestSurrogatePair(t *testing.T) {
	// U+1F600 as a surrogate pair of \uN; lone halves and values out of
	// range are U+FFFD.
	for cp, want := range map[string]string{"1252": "&#128512;", "65001": "\xf0\x9f\x98\x80"} {
		src := `{\rtf1\ansicpg` + cp + `\fromhtml1 {\*\htmltag0 <p>}` + uw + `-10179?` + uw + `-8704?{\*\htmltag0 </p>}}`
		res, _ := Extract(context.Background(), strings.NewReader(src), 1<<20, 10)
		if got := string(res.HTML); got != "<p>"+want+"</p>" {
			t.Errorf("cp %s: html %q", cp, got)
		}
	}
	src := `{\rtf1\fromhtml1 {\*\htmltag0 <p>}` + uw + `-10179?x` + uw + `-8704?` + uw + `-10179?` + uw + `-2147483648?{\*\htmltag0 </p>}` + uw + `-10179?}`
	res, _ := Extract(context.Background(), strings.NewReader(src), 1<<20, 10)
	if got, want := string(res.HTML), "<p>&#65533;x&#65533;&#65533;&#65533;</p>&#65533;"; got != want {
		t.Errorf("html %q, want %q", got, want)
	}
}

func TestLateFromHTML(t *testing.T) {
	// \fromhtml1 after content is not in the header.
	for _, src := range []string{`{\rtf1\~\fromhtml1 {\*\htmltag0 <p>}}`, `{\rtf1\emdash\fromhtml1 {\*\htmltag0 <p>}}`} {
		res, err := Extract(context.Background(), strings.NewReader(src), 1<<20, 10)
		if err != nil || res.Found {
			t.Errorf("%s: res %+v err %v", src, res, err)
		}
	}
}
