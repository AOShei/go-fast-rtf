package gofastrtf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AOShei/go-fast-core"
)

// TestCorpus runs the extractor over the public corpus (GOFAST_CORPUS, or
// ../.testdata/corpus): every RTF file under dev/ and heldout/, each with
// a time limit and bounded budgets. It logs one tally per set (counts
// only: held-out files are scored, never looked at) and fails on a panic,
// a Validate failure or a timeout. With -v it also logs each hidden flag
// found in dev/ files, with an excerpt, for judging false positives.
func TestCorpus(t *testing.T) {
	root := os.Getenv("GOFAST_CORPUS")
	if root == "" {
		root = filepath.Join("..", ".testdata", "corpus")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skip("no corpus at", root)
	}
	for _, set := range []string{"dev", "heldout"} {
		tally := newTally()
		dir := filepath.Join(root, set, "rtf")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			res := corpusOne(path)
			tally.add(res)
			label := path
			if set == "heldout" {
				label = "rtf (held-out file)"
			}
			switch {
			case res.panic != "":
				t.Errorf("%s: panic: %s", label, firstLine(res.panic, set))
			case res.timeout:
				t.Errorf("%s: no result within the time limit", label)
			case res.invalid != nil:
				t.Errorf("%s: Validate: %s", label, firstLine(res.invalid.Error(), set))
			}
			if set == "dev" && testing.Verbose() && res.doc != nil {
				logFlags(t, e.Name(), res.doc)
			}
		}
		t.Logf("%s:\n%s", set, tally)
	}
}

func firstLine(s, set string) string {
	if set == "heldout" {
		// A message can quote the file; held-out files are not shown.
		if i := strings.IndexByte(s, ':'); i > 0 {
			return s[:i]
		}
	}
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

type corpusResult struct {
	doc     *gofastcore.Document
	err     error
	panic   string
	timeout bool
	invalid error
}

func corpusOne(path string) corpusResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan corpusResult, 1)
	go func() {
		var res corpusResult
		defer func() {
			if p := recover(); p != nil {
				res.panic = fmt.Sprintf("%v\n%s", p, debug.Stack())
			}
			done <- res
		}()
		// Embedded objects are handed to a dispatcher that reads them
		// through and declines them, so that the object path runs.
		child := func(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
			return nil, gofastcore.ErrUnsupported
		}
		opts := &gofastcore.Options{Spans: true, Images: gofastcore.ImagesData, Child: child,
			Limits: gofastcore.Limits{MaxTextBytes: 32 << 20, MaxDecodedBytes: 128 << 20}}
		res.doc, res.err = ExtractFile(ctx, path, opts)
		if res.err == nil {
			res.invalid = res.doc.Validate()
		}
	}()
	select {
	case res := <-done:
		if errors.Is(res.err, context.DeadlineExceeded) {
			res.timeout = true
		}
		return res
	case <-time.After(15 * time.Second):
		return corpusResult{timeout: true}
	}
}

type tally struct {
	files    int
	counts   map[string]int
	reasons  map[string]int
	warnings map[string]int
}

func newTally() *tally {
	return &tally{counts: map[string]int{}, reasons: map[string]int{}, warnings: map[string]int{}}
}

func (t *tally) add(r corpusResult) {
	t.files++
	switch {
	case r.panic != "":
		t.counts["panic"]++
	case r.timeout:
		t.counts["timeout"]++
	case r.err != nil:
		kind := "other"
		for _, e := range []error{gofastcore.ErrMalformed, gofastcore.ErrUnsupported, gofastcore.ErrPasswordRequired,
			gofastcore.ErrWrongPassword, gofastcore.ErrLimitExceeded} {
			if errors.Is(r.err, e) {
				kind = strings.TrimPrefix(e.Error(), "gofastcore: ")
			}
		}
		t.counts["error: "+kind]++
	case r.invalid != nil:
		t.counts["validate failure"]++
	default:
		t.counts["ok"]++
	}
	if r.doc == nil {
		return
	}
	var inFile gofastcore.Hidden
	for _, b := range r.doc.AllBlocks() {
		if b.Hidden == 0 {
			continue
		}
		inFile |= b.Hidden
		for bit := gofastcore.Hidden(1); bit != 0; bit <<= 1 {
			if b.Hidden&bit != 0 {
				t.reasons[bit.String()]++
			}
		}
	}
	for bit := gofastcore.Hidden(1); bit != 0; bit <<= 1 {
		if inFile&bit != 0 {
			t.reasons["files "+bit.String()]++
		}
	}
	for _, w := range r.doc.AllWarnings() {
		t.warnings[w.Code]++
	}
}

func (t *tally) String() string {
	var sb strings.Builder
	line := func(title string, m map[string]int) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&sb, "  %s:", title)
		for _, k := range keys {
			fmt.Fprintf(&sb, " %s=%d", k, m[k])
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "  files: %d\n", t.files)
	line("results", t.counts)
	line("hidden blocks by reason", t.reasons)
	line("warnings by code", t.warnings)
	return sb.String()
}

// logFlags logs each hidden flag of a dev/ document with an excerpt.
func logFlags(t *testing.T, name string, doc *gofastcore.Document) {
	for part, b := range doc.AllBlocks() {
		if b.Hidden == 0 {
			continue
		}
		text := b.Text
		for _, sp := range b.Spans {
			if sp.Hidden != 0 {
				text = b.Text[sp.Start:sp.End]
				break
			}
		}
		if b.Table != nil {
			var sb strings.Builder
			for _, row := range b.Table.Rows {
				sb.WriteString(strings.Join(row, " ") + " ")
			}
			text = sb.String()
		}
		r := []rune(strings.Join(strings.Fields(text), " "))
		if len(r) > 60 {
			r = r[:60]
		}
		t.Logf("FLAG %s part %d %s %s: %q", name, part.Number, b.Kind, b.Hidden, string(r))
	}
}
