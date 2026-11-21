package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.rtf")
	src := `{\rtf1\ansi{\info{\title T}}\pard\outlinelevel0 Head\par\pard Body {\v hidden}\par}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{path}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var doc gofastcore.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Metadata.Title != "T" || len(doc.Parts) != 1 || doc.Source.Name != "a.rtf" {
		t.Fatalf("doc %+v", doc)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr %q", errOut.String())
	}
	if b := doc.Parts[0].Blocks[1]; b.Display != gofastcore.DisplayHidden {
		t.Errorf("block %+v", b)
	}

	out.Reset()
	errOut.Reset()
	if code := run(context.Background(), []string{"-format", "markdown", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "Body hidden") {
		t.Errorf("markdown %q", got)
	}
	out.Reset()
	if code := run(context.Background(), []string{"-format", "markdown", "-exclude-display", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "# Head") || strings.Contains(got, "hidden") {
		t.Errorf("markdown %q", got)
	}

	out.Reset()
	if code := run(context.Background(), []string{"-format", "text", "-part-markers", path}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var plain bytes.Buffer
	if code := run(context.Background(), []string{"-format", "text", path}, &plain, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if out.String() == plain.String() || !strings.Contains(out.String(), "Body hidden") {
		t.Errorf("-part-markers gave %q, without %q", out.String(), plain.String())
	}

	if code := run(context.Background(), []string{filepath.Join(dir, "missing.rtf")}, &out, &errOut); code != 1 {
		t.Errorf("missing file: exit %d", code)
	}
	if code := run(context.Background(), []string{"-format", "xml", path}, &out, &errOut); code != 2 {
		t.Errorf("bad format: exit %d", code)
	}
}

// TestChildrenFlag checks that an embedded RTF file, read by the command's
// dispatcher, is printed after its line in text unless -children=false.
func TestChildrenFlag(t *testing.T) {
	inner := `{\rtf1\ansi\pard Inner words\par}`
	var b []byte
	le32 := func(v uint32) { b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
	var pkg []byte
	pkg = append(pkg, 2, 0)
	pkg = append(pkg, "inner.rtf\x00C:\\inner.rtf\x00"...)
	pkg = append(pkg, 0, 0, 3, 0)
	pkg = append(pkg, 12, 0, 0, 0)
	pkg = append(pkg, `C:\inner.rtf`...)
	pkg = append(pkg, byte(len(inner)), 0, 0, 0)
	pkg = append(pkg, inner...)
	le32(0x0501)
	le32(2)
	le32(8)
	b = append(b, "Package\x00"...)
	le32(0)
	le32(0)
	le32(uint32(len(pkg)))
	b = append(b, pkg...)
	src := `{\rtf1\ansi\pard Outer {\object\objemb{\*\objclass Package}{\*\objdata ` +
		hex.EncodeToString(b) + `}{\result shown}}\par}`
	path := filepath.Join(t.TempDir(), "outer.rtf")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"-format", "text", path}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "Outer shown") || !strings.Contains(got, "[child 1: inner.rtf]") || !strings.Contains(got, "Inner words") {
		t.Errorf("default: %q", got)
	}
	out.Reset()
	if code := run(context.Background(), []string{"-format", "text", "-children=false", path}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	got = out.String()
	if !strings.Contains(got, "Outer shown") || strings.Contains(got, "[child 1") || strings.Contains(got, "Inner words") {
		t.Errorf("-children=false: %q", got)
	}
}

func TestExcludeDisplayFlag(t *testing.T) {
	cases := []struct {
		args []string
		want gofastcore.Display
		err  bool
	}{
		{[]string{"-exclude-display"}, gofastcore.DisplayAny, false},
		{[]string{"-exclude-display=true"}, gofastcore.DisplayAny, false},
		{[]string{"-exclude-display=all"}, gofastcore.DisplayAny, false},
		{[]string{"-exclude-display=hidden"}, gofastcore.DisplayHidden, false},
		{[]string{"-exclude-display=no_ink"}, gofastcore.DisplayNoInk, false},
		{[]string{"-exclude-display=hidden,no_ink"}, gofastcore.DisplayHidden | gofastcore.DisplayNoInk, false},
		{[]string{"-exclude-display=hidden+no_ink"}, gofastcore.DisplayHidden | gofastcore.DisplayNoInk, false},
		{[]string{"-exclude-display="}, 0, false},
		{[]string{"-exclude-display=false"}, 0, false},
		{[]string{"-exclude-display=unknown"}, 0, true},
		{[]string{"-exclude-display=hidden,unknown"}, 0, true},
		{[]string{"-exclude-display=bogus"}, 0, true},
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
		if err == nil && ro.ExcludeDisplay != c.want {
			t.Errorf("%v: got %v, want %v", c.args, ro.ExcludeDisplay, c.want)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"-exclude-display=unknown", "x.rtf"}, &out, &errOut); code != 2 {
		t.Errorf("=unknown: exit %d", code)
	}
	if !strings.Contains(errOut.String(), "the reasons are hidden, no_ink\n") {
		t.Errorf("=unknown: stderr %q does not list the reasons", errOut.String())
	}
}
