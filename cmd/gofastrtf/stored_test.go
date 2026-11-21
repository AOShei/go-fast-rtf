package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// TestStoredFlag checks every row of the -stored table in COMMAND_FLAGS.md.
func TestStoredFlag(t *testing.T) {
	cases := []struct {
		args  []string
		all   bool
		kinds []string
		err   bool
	}{
		{[]string{"-stored"}, true, nil, false},
		{[]string{"-stored=true"}, true, nil, false},
		{[]string{"-stored=all"}, true, nil, false},
		{[]string{"-stored=false"}, false, nil, false},
		{[]string{"-stored="}, false, nil, false},
		{[]string{"-stored=markup_comment"}, false, []string{"markup_comment"}, false},
		{[]string{"-stored=field_code,macro"}, false, []string{"field_code", "macro"}, false},
		{[]string{"-stored=field_code+after_end"}, false, []string{"field_code", "after_end"}, false},
		{[]string{"-stored=generator,custom_property"}, false, []string{"generator", "custom_property"}, false},
		{[]string{"-stored=bogus"}, false, nil, true},
		{[]string{"-stored=field_code,bogus"}, false, nil, true},
		{[]string{"-stored=,"}, false, nil, true},
	}
	for _, c := range cases {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		rf := gofastcore.AddRenderFlags(fs)
		err := fs.Parse(c.args)
		var ro gofastcore.RenderOptions
		rf.Apply(&ro)
		if (err != nil) != c.err {
			t.Errorf("%v: err %v", c.args, err)
			continue
		}
		if err == nil && (ro.Stored != c.all || !slices.Equal(ro.StoredKinds, c.kinds)) {
			t.Errorf("%v: got %v %v, want %v %v", c.args, ro.Stored, ro.StoredKinds, c.all, c.kinds)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"-stored=bogus", "x.rtf"}, &out, &errOut); code != 2 {
		t.Errorf("=bogus: exit %d", code)
	}
	if !strings.Contains(errOut.String(), "the kinds are pivot_cache") || !strings.Contains(errOut.String(), "field_code") ||
		!strings.Contains(errOut.String(), "custom_property") {
		t.Errorf("=bogus: stderr %q does not list the kinds", errOut.String())
	}
}

// TestStoredFieldCode: the default text has a field's result and not its
// instruction; -stored=field_code shows the instruction and index entry,
// and leaves out other stored kinds.
func TestStoredFieldCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.rtf")
	rtf := `{\rtf1\ansi{\info{\*\company Acme}}{\*\generator Gen 1;}` +
		`\pard Page {\field{\*\fldinst PAGE }{\fldrslt 7}} here{\xe{Term}}\par}`
	if err := os.WriteFile(path, []byte(rtf), 0o600); err != nil {
		t.Fatal(err)
	}
	text := func(args ...string) string {
		var out, errOut bytes.Buffer
		if code := run(context.Background(), append(append([]string{"-format", "text", "-quiet"}, args...), path), &out, &errOut); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errOut.String())
		}
		return out.String()
	}
	if got := text(); !strings.Contains(got, "Page 7 here") || strings.Contains(got, "PAGE") || strings.Contains(got, "XE") {
		t.Errorf("default: %q", got)
	}
	if got := text("-stored=field_code"); !strings.Contains(got, "Page 7 here") || !strings.Contains(got, "PAGE") ||
		!strings.Contains(got, "XE Term") || strings.Contains(got, "Gen 1") {
		t.Errorf("-stored=field_code: %q", got)
	}
	if got := text("-stored"); !strings.Contains(got, "PAGE") || !strings.Contains(got, "Gen 1") {
		t.Errorf("-stored: %q", got)
	}
}
