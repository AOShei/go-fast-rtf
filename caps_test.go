package gofastrtf

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// limitWarnings returns the messages of the limit.exceeded warnings in ws.
func limitWarnings(ws []gofastcore.Warning) []string {
	var out []string
	for _, w := range ws {
		if w.Code == gofastcore.WarnLimitExceeded {
			if w.Severity != gofastcore.SeverityError {
				panic("limit.exceeded with severity " + w.Severity.String())
			}
			out = append(out, w.Message)
		}
	}
	return out
}

// wantCut checks that ws holds exactly one limit.exceeded warning whose
// message contains each of parts.
func wantCut(t *testing.T, where string, ws []gofastcore.Warning, parts ...string) {
	t.Helper()
	n := 0
	for _, m := range limitWarnings(ws) {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(m, p)
		}
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%s: want one limit.exceeded with %q, got %q", where, parts, limitWarnings(ws))
	}
}

// part returns the part of the given kind, or nil.
func part(doc *gofastcore.Document, kind gofastcore.PartKind) *gofastcore.Part {
	for i := range doc.Parts {
		if doc.Parts[i].Kind == kind {
			return &doc.Parts[i]
		}
	}
	return nil
}

// TestNoCapNoWarning checks that a document that reaches no bound has no
// limit.exceeded anywhere.
func TestNoCapNoWarning(t *testing.T) {
	src := wrap(`{\*\generator Gen 1.0;}{\info{\title T}}{\*\docvar {n}{v}}{\*\userprops{\propname p}{\staticval v}}` +
		`\pard A {\field{\*\fldinst HYPERLINK "x"}{\fldrslt r}}\par`)
	doc := extract(t, src, nil)
	if got := limitWarnings(doc.Warnings); got != nil {
		t.Errorf("document: %q", got)
	}
	for _, p := range doc.Parts {
		if got := limitWarnings(p.Warnings); got != nil {
			t.Errorf("%s: %q", p.Kind, got)
		}
	}
}

// TestFieldInstructionCut checks that field instructions cut at their
// bound give one warning on the body, with the bytes left out.
func TestFieldInstructionCut(t *testing.T) {
	src := wrap(`\pard A {\field{\*\fldinst ` + strings.Repeat("x", 100_000) + `}{\fldrslt r}}` +
		` {\field{\*\fldinst ` + strings.Repeat("y", 40_000) + `}{\fldrslt r}}` +
		` {\xe ` + strings.Repeat("z", 40_000) + `}\par`)
	doc := extract(t, src, nil)
	body := doc.Parts[0]
	wantCut(t, "body", body.Warnings, "field instructions are cut at 32768 bytes", "2 cut", "74464 bytes left out")
	wantCut(t, "body", body.Warnings, "index and table-of-contents entries are cut at 32768 bytes", "1 cut")
}

// TestStoredItemsCut checks the bounds of what goes in the stored part:
// the generator, document variables, custom properties and the text after
// the end; and the metadata on the document.
func TestStoredItemsCut(t *testing.T) {
	src := wrap(`{\*\generator `+strings.Repeat("g", 10_000)+`;}`+
		`{\info{\title `+strings.Repeat("t", 70_000)+`}{\author `+strings.Repeat("a", 70_000)+`}}`+
		`{\*\docvar {`+strings.Repeat("n", 2000)+`}{`+strings.Repeat("v", 70_000)+`}}`+
		`{\*\userprops{\propname `+strings.Repeat("p", 2000)+`}{\staticval `+strings.Repeat("s", 70_000)+`}}`+
		`\pard Body\par`) + strings.Repeat("z", 1<<20+100)
	doc := extract(t, src, nil)
	wantCut(t, "document", doc.Warnings, `metadata items (\info) are cut at 65536 bytes`, "2 cut")
	stored := part(doc, gofastcore.PartStored)
	if stored == nil {
		t.Fatalf("no stored part: %s", dump(doc))
	}
	for _, what := range []string{
		`the generator (\*\generator) is cut at 4096 bytes`,
		"document variable names are cut at 1024 bytes",
		"document variable values are cut at 65536 bytes",
		"custom property names are cut at 1024 bytes",
		"custom property values are cut at 65536 bytes",
		"the text after the end of the document is cut at 1048576 bytes",
	} {
		wantCut(t, "stored", stored.Warnings, what, "1 cut")
	}
	if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
		t.Errorf("body: %q", got)
	}

	// Without the stored part, its warnings are on the document.
	doc = extract(t, src, &gofastcore.Options{Parts: gofastcore.PartRange{First: 1, Last: 1}})
	if part(doc, gofastcore.PartStored) != nil {
		t.Fatal("stored part given")
	}
	wantCut(t, "document", doc.Warnings, `the generator (\*\generator) is cut at 4096 bytes`)
}

// TestStoredItemCount checks that document variables and custom
// properties past maxStoredItems are reported, and that the value of a
// custom property that is not kept does not go to the one before it.
func TestStoredItemCount(t *testing.T) {
	var b strings.Builder
	for i := range 1<<14 + 3 {
		fmt.Fprintf(&b, `{\*\docvar {d%d}{v}}`, i)
	}
	b.WriteString(`{\*\userprops`)
	for i := range 1<<14 + 2 {
		fmt.Fprintf(&b, `{\propname p%d}{\staticval v%d}`, i, i)
	}
	b.WriteString(`}\pard Body\par`)
	doc := extract(t, wrap(b.String()), nil)
	stored := part(doc, gofastcore.PartStored)
	if stored == nil {
		t.Fatal("no stored part")
	}
	wantCut(t, "stored", stored.Warnings, "document variables past 16384 were not read (3)")
	wantCut(t, "stored", stored.Warnings, "custom properties past 16384 were not read (2)")
	last := ""
	for _, bl := range stored.Blocks {
		if bl.Attrs[gofastcore.AttrStored] == gofastcore.StoredCustomProperty {
			last = bl.Text
		}
	}
	if want := fmt.Sprintf("p%d: v%d", 1<<14-1, 1<<14-1); last != want {
		t.Errorf("last custom property %q, want %q", last, want)
	}
}

// TestTableRowsCut checks that rows past maxRows are reported once.
func TestTableRowsCut(t *testing.T) {
	if testing.Short() {
		t.Skip("reads a table of a million rows")
	}
	src := wrap(`\trowd\cellx100` + strings.Repeat(`\intbl\cell\row`, 1<<20+5) + `\pard After\par`)
	doc := extract(t, src, nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "table rows past 1048576 in one table were not read (5)")
}

// TestCellDefinitionsCut checks that \cellx past 4096 in a row is
// reported, and that the text of the cells past it is still read.
func TestCellDefinitionsCut(t *testing.T) {
	src := wrap(`\trowd` + strings.Repeat(`\cellx100`, 4100) + `\pard\intbl` +
		strings.Repeat(`a\cell `, 4099) + `last\cell\row\pard After\par`)
	doc := extract(t, src, nil)
	wantCut(t, "body", doc.Parts[0].Warnings, `cell definitions (\cellx) past 4096 in one row were not read`, "(4)")
	var tb *gofastcore.Table
	for _, bl := range doc.Parts[0].Blocks {
		if bl.Kind == gofastcore.KindTable {
			tb = bl.Table
		}
	}
	if tb == nil || len(tb.Rows) != 1 || len(tb.Rows[0]) != 4100 || tb.Rows[0][4099] != "last" {
		t.Fatalf("the cells past 4096 should be read")
	}
}

// TestBodyBuffersCut checks the smaller bounds of text that goes to the
// body: tracked deletions, comment authors, form field choices, nested
// fields.
func TestBodyBuffersCut(t *testing.T) {
	var choices strings.Builder
	for i := range 1030 {
		fmt.Fprintf(&choices, `{\*\ffl c%d}`, i)
	}
	nested := strings.Repeat(`{\field{\fldrslt `, 260) + "deep" + strings.Repeat(`}}`, 260)
	src := wrap(`\pard A{\deleted ` + strings.Repeat("d", 1<<20+10) + `} B\par` +
		`\pard C{\*\atnauthor ` + strings.Repeat("u", 2000) + `}\chatn{\*\annotation note}\par` +
		`\pard {\field{\*\fldinst {\*\formfield{\*\ffname ` + strings.Repeat("f", 5000) + `}` + choices.String() +
		`{\*\ffl ` + strings.Repeat("c", 5000) + `}}}{\fldrslt x}}\par` +
		`\pard ` + nested + `\par`)
	doc := extract(t, src, nil)
	ws := doc.Parts[0].Warnings
	wantCut(t, "body", ws, "tracked deletions are cut at 1048576 bytes", "1 cut", "10 bytes left out")
	wantCut(t, "body", ws, "comment authors are cut at 1024 bytes")
	wantCut(t, "body", ws, "form field names are cut at 4096 bytes")
	wantCut(t, "body", ws, "form field choices past 1024 in one field were not read (7)")
	wantCut(t, "body", ws, "fields nested deeper than 256 were not read (1)")
}

// TestUnmappedAggregated checks that text.unmapped_glyphs is one warning
// for the part whatever the number of code pages, with the total, the
// first eight named and the rest counted; and that with one code page the
// message is the one it always was.
func TestUnmappedAggregated(t *testing.T) {
	var b strings.Builder
	for cp := range 5000 {
		fmt.Fprintf(&b, `{\ansicpg%d \'e9}`, 20000+cp)
	}
	// The font names no character set, so text is in the \ansicpg.
	plain := func(body string) string { return `{\rtf1\ansi\deff0{\fonttbl{\f0 Times;}}` + body + `}` }
	doc := extract(t, plain(`\pard `+b.String()+`\par`), nil)
	var msgs []string
	for _, w := range doc.Parts[0].Warnings {
		if w.Code == gofastcore.WarnTextUnmappedGlyphs {
			msgs = append(msgs, w.Message)
		}
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d text.unmapped_glyphs warnings", len(msgs))
	}
	for _, want := range []string{"5000 characters or bytes in all are U+FFFD: ", "code page 20000 was not decoded: 1 bytes are U+FFFD; ", "code page 20007 was not decoded", "; and at least 57 more code pages (past the first 64 met they are not told apart), with 4992 more U+FFFD"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("message %q lacks %q", msgs[0], want)
		}
	}
	if strings.Contains(msgs[0], "20008") {
		t.Errorf("message names more than eight code pages: %q", msgs[0])
	}

	doc = extract(t, plain(`\pard {\ansicpg20000 \'e9\'e9}\par`), nil)
	if ws := doc.Parts[0].Warnings; len(ws) == 0 || ws[0].Code != gofastcore.WarnTextUnmappedGlyphs || ws[0].Message != "code page 20000 was not decoded: 2 bytes are U+FFFD" {
		t.Errorf("one code page: %+v", ws)
	}
}

// TestCutAtTheBound checks that a buffer holds exactly its bound: text
// that fills it is not cut, and one byte more is.
func TestCutAtTheBound(t *testing.T) {
	for _, n := range []int{32766, 32768} {
		doc := extract(t, wrap(`\pard A {\field{\*\fldinst `+strings.Repeat("x", n)+`}{\fldrslt r}}\par`), nil)
		if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
			t.Errorf("%d bytes: %q", n, got)
		}
		found := false
		for _, b := range doc.Parts[0].Blocks {
			found = found || b.Kind == gofastcore.KindNote && len(b.Text) == n
		}
		if !found {
			t.Errorf("%d bytes: no field note of that length", n)
		}
	}
	doc := extract(t, wrap(`\pard A {\field{\*\fldinst `+strings.Repeat("x", 32769)+`}{\fldrslt r}}\par`), nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "field instructions are cut at 32768 bytes: 1 cut, 1 bytes left out")
	// A rune that does not fit ends the text, though a shorter one would.
	doc = extract(t, wrap(`\pard A {\field{\*\fldinst `+strings.Repeat("x", 32766)+`\u8364?y}{\fldrslt r}}\par`), nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "field instructions are cut at 32768 bytes: 1 cut, 4 bytes left out")
}

// TestObjectNameCut checks that a cut \objname is reported only when it is
// the child's name, not when a Package's label replaces it.
func TestObjectNameCut(t *testing.T) {
	long := strings.Repeat("o", 2000)
	doc := extract(t, wrap(`\pard {\object\objemb{\*\objname `+long+`}{\*\objclass Package}{\*\objdata `+packageHex()+`}{\result r}}\par`), nil)
	if got := limitWarnings(doc.Warnings); got != nil {
		t.Errorf("name replaced by the label: %q", got)
	}
	doc = extract(t, wrap(`\pard {\object\objemb{\*\objname `+long+`}{\*\objclass Package}{\result r}}\par`), nil)
	wantCut(t, "document", doc.Warnings, `embedded object names (\objname) are cut at 1024 bytes`)
}

// TestDescriptionCutOnce checks that of several wzDescription values on
// one picture only the one kept is reported.
func TestDescriptionCutOnce(t *testing.T) {
	png := "89504e470d0a1a0a0000000d49484452000000100000000808060000001f15c489"
	sp := `{\sp{\sn wzDescription}{\sv ` + strings.Repeat("d", 70_000) + `}}`
	doc := extract(t, wrap(`\pard {\pict{\*\picprop`+sp+sp+sp+`}\pngblip `+png+`}\par`), nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "picture and shape descriptions (wzDescription) are cut at 65536 bytes: 1 cut")
}

// TestDefinitionsCut checks the bounds of the font table, the style
// sheet, the list table and the list override table.
func TestDefinitionsCut(t *testing.T) {
	if testing.Short() {
		t.Skip("reads tables of 65,536 entries")
	}
	var fonts, styles, lists, overrides strings.Builder
	for i := range 1<<16 + 2 {
		if i < 1<<16-1 {
			fmt.Fprintf(&fonts, `{\f%d F;}`, i+10)
		} else {
			// Past the bound, an entry with two \f is counted once.
			fmt.Fprintf(&fonts, `{\f%d\f%d F;}`, i+10, i+10+(1<<20))
		}
		fmt.Fprintf(&styles, `{\s%d S;}`, i+10)
		fmt.Fprintf(&lists, `{\list\listid%d}`, i+1)
		fmt.Fprintf(&overrides, `{\listoverride\listid1\ls%d}`, i+1)
	}
	src := `{\rtf1\ansi\deff0{\fonttbl{\f0 Times;}` + fonts.String() + `}{\stylesheet{\s0 Normal;}` + styles.String() + `}` +
		`{\*\listtable` + lists.String() + `}{\*\listoverridetable` + overrides.String() + `}\pard Body\par}`
	doc := extract(t, src, nil)
	ws := doc.Parts[0].Warnings
	wantCut(t, "body", ws, "fonts past 65536 in the font table were not read", "(3)")
	wantCut(t, "body", ws, "styles past 65536 in the style sheet were not read", "(3)")
	wantCut(t, "body", ws, "lists past 65536 in the list table were not read", "(2)")
	wantCut(t, "body", ws, `list overrides (\ls) past 65536 were not read`, "(2)")

	// A font table whose entries are not in groups: each ";" ends one.
	fonts.Reset()
	for i := range 1<<16 + 4 {
		fmt.Fprintf(&fonts, `\f%d F;`, i)
	}
	doc = extract(t, `{\rtf1\ansi\deff0{\fonttbl`+fonts.String()+`}\pard Body\par}`, nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "fonts past 65536 in the font table were not read", "(4)")
}

// TestStoredTextCutWithoutStoredPart checks that the text limit reached in
// the stored data is reported on the document when the stored part is
// not given.
func TestStoredTextCutWithoutStoredPart(t *testing.T) {
	src := wrap(`{\*\generator ` + strings.Repeat("g", 3000) + `;}\pard Body\par`)
	lim := gofastcore.Limits{MaxTextBytes: 2000}
	const msg = "the text limit was reached; the stored data is cut off there"
	doc := extract(t, src, &gofastcore.Options{Limits: lim})
	if part(doc, gofastcore.PartStored) != nil {
		t.Fatalf("stored part given: %s", dump(doc))
	}
	wantCut(t, "document", doc.Warnings, msg)
	// The body is complete: it carries no limit.exceeded.
	if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
		t.Errorf("body: %q", got)
	}
	if bs := blocks(doc); len(bs) != 1 || bs[0].Text != "Body" {
		t.Errorf("body blocks: %s", dump(doc))
	}
	// The repro of the review, without wrap's tables.
	doc = extract(t, `{\rtf1\ansi{\fonttbl{\f0 Times;}}{\*\generator `+strings.Repeat("g", 3000)+`;}\pard Body\par}`, &gofastcore.Options{Limits: lim})
	if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
		t.Errorf("body: %q", got)
	}
	for _, o := range []*gofastcore.Options{
		{Limits: lim, Parts: gofastcore.PartRange{First: 1, Last: 1}},
		{Limits: gofastcore.Limits{MaxTextBytes: 2000, MaxParts: 1}},
	} {
		doc = extract(t, src, o)
		wantCut(t, "document", doc.Warnings, msg)
		if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
			t.Errorf("body: %q", got)
		}
	}
}

// TestUnmappedFewCodePages checks the message for a few code pages, all
// counted one by one.
func TestUnmappedFewCodePages(t *testing.T) {
	var b strings.Builder
	for cp := range 20 {
		fmt.Fprintf(&b, `{\ansicpg%d \'e9}`, 20000+cp)
	}
	doc := extract(t, `{\rtf1\ansi\deff0{\fonttbl{\f0 Times;}}\pard `+b.String()+`\par}`, nil)
	n := 0
	for _, w := range doc.Parts[0].Warnings {
		if w.Code == gofastcore.WarnTextUnmappedGlyphs {
			n++
			if !strings.HasPrefix(w.Message, "20 characters or bytes in all are U+FFFD: code page 20000 was not decoded: 1 bytes are U+FFFD; ") ||
				!strings.HasSuffix(w.Message, "; and 12 more code pages, with 12 more U+FFFD") {
				t.Errorf("message %q", w.Message)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d warnings", n)
	}
}

// TestDeletionCutAtTheBound checks that a tracked deletion holds exactly
// its bound, and that the text is cut at the first rune that does not fit.
func TestDeletionCutAtTheBound(t *testing.T) {
	doc := extract(t, wrap(`\pard A{\deleted `+strings.Repeat("d", 1<<20)+`} B\par`), nil)
	if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
		t.Errorf("deletion of exactly the bound: %q", got)
	}
	doc = extract(t, wrap(`\pard A{\deleted `+strings.Repeat("d", 1<<20-1)+`\u8364?e} B\par`), nil)
	wantCut(t, "body", doc.Parts[0].Warnings, "tracked deletions are cut at 1048576 bytes: 1 cut, 4 bytes left out")
}

// TestCutOfWhiteSpace checks that a buffer cut where it holds only white
// space is reported, though what is kept is empty once trimmed: the text
// after the bound is lost all the same.
func TestCutOfWhiteSpace(t *testing.T) {
	ws := strings.Repeat(" ", 70_000)
	src := wrap(`{\info{\title ` + ws + `Title}}{\*\docvar {n}{` + ws + `v}}` +
		`\pard A{\deleted ` + strings.Repeat(" ", 1<<20+10) + `gone} {\field{\*\fldinst ` + strings.Repeat(" ", 40_000) + `PAGE}{\fldrslt 1}}\par`)
	doc := extract(t, src, nil)
	if doc.Metadata.Title != "" {
		t.Fatalf("title %q", doc.Metadata.Title)
	}
	wantCut(t, "document", doc.Warnings, `metadata items (\info) are cut at 65536 bytes`)
	wantCut(t, "body", doc.Parts[0].Warnings, "tracked deletions are cut at 1048576 bytes")
	wantCut(t, "body", doc.Parts[0].Warnings, "field instructions are cut at 32768 bytes")
	// The variable's name is kept, so there is a stored part.
	stored := part(doc, gofastcore.PartStored)
	if stored == nil {
		t.Fatal("no stored part")
	}
	wantCut(t, "stored", stored.Warnings, "document variable values are cut at 65536 bytes")
}

// TestCommentDateCut checks that a comment date (\atndate) longer than
// its bound is reported: its number is cut, and so is the date given.
func TestCommentDateCut(t *testing.T) {
	doc := extract(t, wrap(`\pard C\chatn{\*\annotation{\*\atndate `+strings.Repeat("0", 40)+`1}note}\par`), nil)
	wantCut(t, "body", doc.Parts[0].Warnings, `comment dates (\atndate) are cut at 32 bytes`)
	doc = extract(t, wrap(`\pard C\chatn{\*\annotation{\*\atndate 1234567890}note}\par`), nil)
	if got := limitWarnings(doc.Parts[0].Warnings); got != nil {
		t.Errorf("a date of ordinary length: %q", got)
	}
}

// TestPackageLabelCut checks that the label of a packaged file, which
// names the child, is cut at its bound with a warning on the document,
// and that a long original path, which is not given, does not stop the
// file from being unpacked.
func TestPackageLabelCut(t *testing.T) {
	var name, data string
	child := func(ctx context.Context, n, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
		buf := make([]byte, size)
		r.ReadAt(buf, 0)
		name, data = n, string(buf)
		d := gofastcore.New("txt")
		p := gofastcore.Part{Kind: gofastcore.PartBody, Number: 1}
		p.Finish()
		d.Parts = append(d.Parts, p)
		return d, nil
	}
	obj := func(hex string) string {
		return wrap(`\pard {\object\objemb{\*\objclass Package}{\*\objdata ` + hex + `}{\result r}}\par`)
	}
	doc := extract(t, obj(packageHexWith(strings.Repeat("L", 5000), `C:\x.txt`)), &gofastcore.Options{Child: child})
	if name != strings.Repeat("L", 4096) || data != "hello" {
		t.Errorf("child %d bytes of name, data %q", len(name), data)
	}
	wantCut(t, "document", doc.Warnings, "embedded file labels (Package) are cut at 4096 bytes: 1 cut, 904 bytes left out")

	doc = extract(t, obj(packageHexWith("note.txt", `C:\`+strings.Repeat("p", 5000))), &gofastcore.Options{Child: child})
	if name != "note.txt" || data != "hello" {
		t.Errorf("long path: child %q, data %q", name, data)
	}
	if got := limitWarnings(doc.Warnings); got != nil {
		t.Errorf("long path: %q", got)
	}
}

// TestLargePNGSize checks that a PNG's size is given up to the largest
// its specification allows.
func TestLargePNGSize(t *testing.T) {
	png := "89504e470d0a1a0a0000000d49484452" + "02000000" + "00000008" + "08060000001f15c489"
	doc := extract(t, wrap(`\pard {\pict\pngblip `+png+`}\par`), nil)
	var img *gofastcore.Image
	for _, b := range doc.Parts[0].Blocks {
		if b.Image != nil {
			img = b.Image
		}
	}
	if img == nil || img.Width != 1<<25 || img.Height != 8 {
		t.Errorf("image %+v", img)
	}
}

// TestLargeUnicodeSkip checks that \ucN above 16 skips N fallback bytes
// after each \uN, so that none of them is given as text.
func TestLargeUnicodeSkip(t *testing.T) {
	fallback := strings.Repeat("?", 20)
	doc := extract(t, wrap(`\pard {\uc20 a\u8364`+fallback+`b\u8364\'3f\'3f`+strings.Repeat("?", 18)+`c}\par`), nil)
	if got, want := blocks(doc)[0].Text, "a\u20acb\u20acc"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
}
