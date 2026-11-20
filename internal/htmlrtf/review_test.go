package htmlrtf

import (
	"context"
	"strings"
	"testing"
)

func TestLateFromHTML(t *testing.T) {
	// \fromhtml1 after content is not in the header.
	for _, src := range []string{`{\rtf1\~\fromhtml1 {\*\htmltag0 <p>}}`, `{\rtf1\emdash\fromhtml1 {\*\htmltag0 <p>}}`} {
		res, err := Extract(context.Background(), strings.NewReader(src), 1<<20, 10)
		if err != nil || res.Found {
			t.Errorf("%s: res %+v err %v", src, res, err)
		}
	}
}
