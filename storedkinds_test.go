package gofastrtf_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
	gofastrtf "github.com/AOShei/go-fast-rtf"
)

// Every Attrs["stored"] value the library writes is in core's
// StoredKindNames, so a command can accept it in -stored=kinds.
func TestStoredKindNamesComplete(t *testing.T) {
	src := []byte(`{\rtf1{\*\generator Test Writer 1.0;}{\*\docvar {Var}{value}}` +
		`{\*\userprops {\propname Client}\proptype30{\staticval ACME}}` +
		`See {\field{\*\fldinst PAGE}{\fldrslt 7}}.}trailing`)
	doc, err := gofastrtf.Extract(context.Background(), bytes.NewReader(src), int64(len(src)), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, b := range doc.AllBlocks() {
		if k := b.Attrs[gofastcore.AttrStored]; k != "" {
			seen[k] = true
			if !slices.Contains(gofastcore.StoredKindNames(), k) {
				t.Errorf("stored kind %q is not listed", k)
			}
		}
	}
	for _, k := range []string{gofastrtf.StoredGenerator, gofastrtf.StoredCustomProperty} {
		if !seen[k] {
			t.Errorf("listed kind %q not produced by the fixture", k)
		}
	}
}
