package main

import (
	"bytes"
	"context"
	"encoding/json"
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

	if code := run(context.Background(), []string{filepath.Join(dir, "missing.rtf")}, &out, &errOut); code != 1 {
		t.Errorf("missing file: exit %d", code)
	}
	if code := run(context.Background(), []string{"-format", "xml", path}, &out, &errOut); code != 2 {
		t.Errorf("bad format: exit %d", code)
	}
}
