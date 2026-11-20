package htmlrtf

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNested(t *testing.T) {
	// htmltag groups nested deeper than the limit are not read.
	src := `{\rtf1\fromhtml1 ` + strings.Repeat(`{\*\htmltag0 <b>`, 100) + strings.Repeat(`}`, 100) + `}`
	res, err := Extract(context.Background(), strings.NewReader(src), 1<<20, 10)
	if err != nil || !res.Found || string(res.HTML) != strings.Repeat("<b>", 9) {
		t.Errorf("res %+v err %v", res, err)
	}
}

func TestCut(t *testing.T) {
	src := `{\rtf1\fromhtml1 {\*\htmltag0 0123456789}}`
	res, _ := Extract(context.Background(), strings.NewReader(src), 5, 10)
	if !res.Found || !res.Cut || res.HTML != nil {
		t.Errorf("res %+v", res)
	}
}

func FuzzExtract(f *testing.F) {
	for _, s := range []string{
		`{\rtf1\ansi\ansicpg1252\fromhtml1 {\fonttbl{\f0 A;}}{\*\htmltag19 <html>}\htmlrtf {\b\htmlrtf0 a\'e9\par\tab\{\}\\\htmlrtf}x\htmlrtf0 {\*\htmltag27 </html>}}`,
		`{\rtf1\ansicpg65001\fromhtml1 {\*\htmltag0 <p>}\uc2\` + `u-3913\'3f\'3fz{\*\mhtmltag0 x}{\*\htmltag{\*\htmltag <i>}}\bin3 abc}`,
		`{\rtf1 text}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		timer := time.AfterFunc(5*time.Second, func() { panic(fmt.Sprintf("slow input: %q", b)) })
		defer timer.Stop()
		res, err := Extract(context.Background(), bytes.NewReader(b), 1<<16, 64)
		if err != nil {
			return
		}
		if len(res.HTML) > 1<<16 || (!res.Found || res.Cut) && res.HTML != nil {
			t.Fatalf("res %+v", res)
		}
	})
}
