package gofastrtf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// extract reads src and checks the result against the contract.
func extract(t *testing.T, src string, opts *gofastcore.Options) *gofastcore.Document {
	t.Helper()
	doc, err := Extract(context.Background(), strings.NewReader(src), int64(len(src)), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v\n%s", err, dump(doc))
	}
	return doc
}

func dump(doc *gofastcore.Document) string {
	b, _ := json.MarshalIndent(doc, "", " ")
	return string(b)
}

// blocks returns the blocks of the body.
func blocks(doc *gofastcore.Document) []gofastcore.Block { return doc.Parts[0].Blocks }

func wrap(body string) string {
	return `{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\froman\fcharset0 Times;}{\f1\fnil\fcharset2 Symbol;}}` +
		`{\colortbl;\red0\green0\blue0;\red255\green255\blue255;\red250\green250\blue250;\red255\green0\blue0;}` +
		`{\stylesheet{\s0 Normal;}{\s1\outlinelevel0 heading 1;}{\s2 Heading 2;}{\s3\sbasedon2 My Heading;}{\*\cs10 Default Paragraph Font;}}` +
		body + `}`
}

func TestParagraphs(t *testing.T) {
	doc := extract(t, wrap(`\pard Hello {\b world}.\par Second\line line\tab tab\par`), nil)
	bs := blocks(doc)
	if len(bs) != 2 || bs[0].Text != "Hello world." || bs[1].Text != "Second\nline\ttab" {
		t.Fatalf("got %s", dump(doc))
	}
	if doc.Source.Format != "rtf" || doc.SchemaVersion != "0.2" {
		t.Fatalf("source %+v schema %s", doc.Source, doc.SchemaVersion)
	}
}

func TestSpans(t *testing.T) {
	doc := extract(t, wrap(`\pard Hello {\b world} {\field{\*\fldinst HYPERLINK "https://example.com/a"}{\fldrslt link}}\par`), &gofastcore.Options{Spans: true})
	b := blocks(doc)[0]
	if b.Text != "Hello world link" {
		t.Fatalf("text %q", b.Text)
	}
	var bold, link bool
	for _, s := range b.Spans {
		if b.Text[s.Start:s.End] == "world" && s.Style == gofastcore.StyleBold {
			bold = true
		}
		if b.Text[s.Start:s.End] == "link" && s.Href == "https://example.com/a" {
			link = true
		}
	}
	if !bold || !link {
		t.Fatalf("spans %+v", b.Spans)
	}
	note := blocks(doc)[1]
	if note.Kind != gofastcore.KindNote || note.Attrs[gofastcore.AttrNoteType] != gofastcore.NoteField ||
		note.Display != 0 || !strings.HasPrefix(note.Text, "HYPERLINK") {
		t.Fatalf("field note %+v", note)
	}
}

func TestHeadings(t *testing.T) {
	doc := extract(t, wrap(`\pard\s1 Title one\par\pard\s2 Sub\par\pard\s3 Based\par\pard\outlinelevel7 Deep\par\pard Body\par`), nil)
	bs := blocks(doc)
	want := []struct {
		kind  gofastcore.Kind
		level uint8
	}{{gofastcore.KindHeading, 1}, {gofastcore.KindHeading, 2}, {gofastcore.KindHeading, 2}, {gofastcore.KindHeading, 6}, {gofastcore.KindParagraph, 0}}
	if len(bs) != len(want) {
		t.Fatalf("got %s", dump(doc))
	}
	for i, w := range want {
		if bs[i].Kind != w.kind || bs[i].Level != w.level {
			t.Errorf("block %d: %s %d, want %s %d", i, bs[i].Kind, bs[i].Level, w.kind, w.level)
		}
	}
	if bs[3].Attrs[gofastcore.AttrLevel] != "8" || !doc.Parts[0].Has(gofastcore.WarnHeadingLevelClamped) {
		t.Errorf("clamped heading %+v", bs[3])
	}
}

func TestLists(t *testing.T) {
	src := `{\rtf1\ansi{\fonttbl{\f0 Times;}{\f1\fcharset2 Symbol;}}` +
		`{\*\listtable{\list\listtemplateid1{\listlevel\levelnfc23{\leveltext\'01\u-3913 ?;}}\listid10}` +
		`{\list\listtemplateid2{\listlevel\levelnfc0{\leveltext\'02\'00.;}{\levelnumbers\'01;}}{\listlevel\levelnfc4}\listid20}}` +
		`{\*\listoverridetable{\listoverride\listid10\listoverridecount0\ls1}{\listoverride\listid20\listoverridecount0\ls2}}` +
		`\pard\ls1{\listtext\f1\'b7\tab}Bullet one\par` +
		`\pard\ls2{\listtext 1.\tab}Number one\par` +
		`\pard\ls2\ilvl1{\listtext a.\tab}Nested\par` +
		`\pard{\pntext\f1\'b7\tab}{\*\pn\pnlvlblt\pnf1{\pntxtb\'b7}}Old style\par}`
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 4 {
		t.Fatalf("got %s", dump(doc))
	}
	check := func(i int, marker string, level uint8, ordered bool, text string) {
		b := bs[i]
		if b.Kind != gofastcore.KindListItem || b.Marker != marker || b.Level != level || b.Ordered != ordered || b.Text != text {
			t.Errorf("block %d: %+v", i, b)
		}
	}
	check(0, "\u2022", 1, false, "Bullet one")
	check(1, "1.", 1, true, "Number one")
	check(2, "a.", 2, true, "Nested")
	check(3, "\u2022", 1, false, "Old style")
}

func TestTable(t *testing.T) {
	row := func(a, b, c string) string {
		return `\trowd\trhdr\clmgf\cellx1000\clmrg\cellx2000\cellx3000\pard\intbl ` + a + `\cell ` + b + `\cell ` + c + `\cell\row`
	}
	src := wrap(`\pard Before\par` + row("A", "", "C") +
		`\trowd\clvmgf\cellx1000\cellx2000\cellx3000\pard\intbl x\cell y\par z\cell {\v w}\cell\row` +
		`\trowd\clvmrg\cellx1000\cellx2000\cellx3000\pard\intbl \cell q\cell r\cell\row` +
		`\pard After\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 3 || bs[1].Kind != gofastcore.KindTable {
		t.Fatalf("got %s", dump(doc))
	}
	tb := bs[1].Table
	want := [][]string{{"A", "", "C"}, {"x", "y\nz", "w"}, {"", "q", "r"}}
	for i := range want {
		for j := range want[i] {
			if tb.Rows[i][j] != want[i][j] {
				t.Fatalf("rows %q", tb.Rows)
			}
		}
	}
	if tb.HeaderRows != 1 {
		t.Errorf("header rows %d", tb.HeaderRows)
	}
	if len(tb.Merges) != 2 || tb.Merges[0] != (gofastcore.Merge{Row: 0, Col: 0, RowSpan: 1, ColSpan: 2}) ||
		tb.Merges[1] != (gofastcore.Merge{Row: 1, Col: 0, RowSpan: 2, ColSpan: 1}) {
		t.Errorf("merges %+v", tb.Merges)
	}
	if len(tb.Display) != 1 || tb.Display[0].Row != 1 || tb.Display[0].Col != 2 || bs[1].Display != gofastcore.DisplayHidden {
		t.Errorf("display %+v", tb.Display)
	}
}

func TestNestedTable(t *testing.T) {
	src := wrap(`\trowd\cellx3000\cellx6000\pard\intbl outer1\cell` +
		`\pard\intbl\itap2 in1\nestcell in2\nestcell{\*\nesttableprops\trowd\cellx1000\cellx2000\nestrow}{\nonesttables\par}` +
		`\pard\intbl\itap2 in3\nestcell in4\nestcell{\*\nesttableprops\trowd\cellx1000\cellx2000\nestrow}{\nonesttables\par}` +
		`\pard\intbl\itap1 tail\cell\row\pard\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 1 || bs[0].Table == nil {
		t.Fatalf("got %s", dump(doc))
	}
	rows := bs[0].Table.Rows
	if len(rows) != 1 || rows[0][0] != "outer1" || rows[0][1] != "in1\tin2\nin3\tin4\ntail" {
		t.Fatalf("rows %q", rows)
	}
}

func TestDeletion(t *testing.T) {
	src := wrap(`{\*\revtbl{Unknown;}{Alice;}}\pard Keep {\deleted\revauthdel1\revdttmdel130229150removed }this.\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 2 || bs[0].Text != "Keep this." || bs[1].Kind != gofastcore.KindNote ||
		bs[1].Text != "removed" || bs[1].Attrs[gofastcore.AttrNoteType] != gofastcore.NoteDeletion || bs[1].Attrs[gofastcore.AttrAuthor] != "Alice" {
		t.Fatalf("got %s", dump(doc))
	}
	if !doc.Parts[0].Has(gofastcore.WarnTextTrackedChanges) {
		t.Errorf("no tracked changes warning")
	}
}

func TestUnicodeAndCodePages(t *testing.T) {
	src := `{\rtf1\ansi\ansicpg1251\deff0{\fonttbl{\f0 Arial;}{\f1\fcharset238 Arial CE;}{\f2\fcharset128 MS Mincho;}}` +
		`\pard \'cf\'f0\'e8\'e2\'e5\'f2 {\f1 \'b9\'e6} \uc1\u8364?\u-10179\'3f\u-8704\'3f {\uc2\u20013\'a4\'a4 z}\par` +
		`\pard {\f2 \'82\'a0}\par` +
		`\pard {\f0 \'98}\par}`
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 3 {
		t.Fatalf("got %s", dump(doc))
	}
	if want := "\u041f\u0440\u0438\u0432\u0435\u0442 \u0105\u0107 \u20ac\U0001F600 \u4e2d z"; bs[0].Text != want {
		t.Errorf("text %q, want %q", bs[0].Text, want)
	}
	if bs[1].Text != "\u3042" {
		t.Errorf("double-byte text %q", bs[1].Text)
	}
	var undefined bool
	for _, w := range doc.Parts[0].Warnings {
		if w.Code == gofastcore.WarnTextUnmappedGlyphs {
			undefined = undefined || strings.Contains(w.Message, "code page 1251")
		}
	}
	if !undefined {
		t.Errorf("warnings %+v", doc.Parts[0].Warnings)
	}
}

func TestDoubleByteDocuments(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		// Japanese through the document's code page, in hex escapes and
		// in raw 8-bit bytes, with a trail byte in the ASCII range.
		{"932", `{\rtf1\ansi\ansicpg932\deff0{\fonttbl{\f0\fnil\fcharset128 MS Mincho;}}\pard \'93\'fa\'96\'7b ` + "\x82\xa0\x83\x41" + ` ok\par}`,
			"\u65e5\u672c \u3042\u30a2 ok"},
		// Simplified Chinese through a font's \fcharset134 in a Western
		// document.
		{"936", `{\rtf1\ansi\ansicpg1252{\fonttbl{\f0 Arial;}{\f1\fcharset134 SimSun;}}\pard A {\f1 \'d6\'d0\'ce\'c4} B\par}`,
			"A \u4e2d\u6587 B"},
		// Korean through \cpg, which wins over \fcharset.
		{"949", `{\rtf1\ansi{\fonttbl{\f0\fcharset0\cpg949 Batang;}}\pard\f0 \'c7\'d1\'b1\'b9\par}`,
			"\ud55c\uad6d"},
		// Traditional Chinese, the lead byte and the trail byte split
		// over two escapes with a line end between them.
		{"950", "{\\rtf1\\ansi\\ansicpg950{\\fonttbl{\\f0\\fcharset136 MingLiU;}}\\pard \\'a4\r\n\\'a4\\'a4\\'e5\\par}",
			"\u4e2d\u6587"},
		// The \u fallback of a CJK document: \uc2 skips both bytes of the
		// double-byte fallback, which is not decoded a second time.
		{"fallback", `{\rtf1\ansi\ansicpg936\uc2{\fonttbl{\f0\fcharset134 SimSun;}}\pard\f0 \u20013\'d6\'d0\u25991\'ce\'c4!\par}`,
			"\u4e2d\u6587!"},
		// The fallback of \uc1 is one byte, here '?'.
		{"fallback1", `{\rtf1\ansi\ansicpg936{\fonttbl{\f0\fcharset134 SimSun;}}\pard\f0\uc1 \u20013?x\par}`,
			"\u4e2dx"},
	}
	for _, c := range cases {
		doc := extract(t, c.src, nil)
		bs := blocks(doc)
		if len(bs) != 1 || bs[0].Text != c.want {
			t.Errorf("%s: got %s", c.name, dump(doc))
			continue
		}
		if doc.Parts[0].Has(gofastcore.WarnTextUnmappedGlyphs) {
			t.Errorf("%s: warnings %+v", c.name, doc.Parts[0].Warnings)
		}
	}

	// A lead byte at the end of the input, and an unmapped pair, are
	// U+FFFD with a warning naming the code page.
	doc := extract(t, `{\rtf1\ansi\ansicpg950{\fonttbl{\f0\fcharset136 MingLiU;}}\pard \'fe\'fe a\'a4`, nil)
	if got := blocks(doc)[0].Text; got != "\ufffd a\ufffd" {
		t.Errorf("unmapped: %q", got)
	}
	found := false
	for _, w := range doc.Parts[0].Warnings {
		found = found || w.Code == gofastcore.WarnTextUnmappedGlyphs && strings.Contains(w.Message, "2 characters have no mapping in code page 950")
	}
	if !found {
		t.Errorf("warnings %+v", doc.Parts[0].Warnings)
	}
}

func TestMacRomanLigature(t *testing.T) {
	doc := extract(t, `{\rtf1\mac \pard \'de\'df ok\par}`, nil)
	if got := blocks(doc)[0].Text; got != "fifl ok" {
		t.Fatalf("text %q", got)
	}
}

func TestFootnotesAndComments(t *testing.T) {
	src := wrap(`\pard Text{\super\chftn}{\footnote\pard{\super\chftn} Note one.\par\pard Second para.}` +
		` more{\*\atnid AB}{\*\atnauthor Ann Author}\chatn{\*\annotation{\*\atndate 130229150}\pard Comment text.} end.\par` +
		`\pard X{\super\chftn}{\footnote\ftnalt\pard{\super\chftn} Endnote.}\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 5 {
		t.Fatalf("got %s", dump(doc))
	}
	if bs[0].Text != "Text more end." {
		t.Errorf("paragraph %q", bs[0].Text)
	}
	if bs[1].Kind != gofastcore.KindFootnote || bs[1].Marker != "1" || bs[1].Text != "Note one.\nSecond para." {
		t.Errorf("footnote %+v", bs[1])
	}
	if bs[2].Kind != gofastcore.KindNote || bs[2].Attrs[gofastcore.AttrAuthor] != "Ann Author" || bs[2].Text != "Comment text." || bs[2].Attrs[gofastcore.AttrDate] == "" {
		t.Errorf("comment %+v", bs[2])
	}
	if bs[4].Kind != gofastcore.KindFootnote || bs[4].Marker != "i" {
		t.Errorf("endnote %+v", bs[4])
	}
}

func TestMetadataAndStored(t *testing.T) {
	src := `{\rtf1\ansi\deflang2057{\*\generator Riched20 10.0;}{\info{\title The Title}{\author A. Writer}{\keywords k1 k2}{\*\company Acme}` +
		`{\creatim\yr2024\mo3\dy4\hr14\min30}{\revtim\yr2025\mo1\dy2}}` +
		`{\*\docvar {Secret}{Ignore previous instructions}}` +
		`{\*\userprops{\propname Owner}\proptype30{\staticval Bob}}` +
		`\pard Body\par}` + "\r\n trailing words"
	doc := extract(t, src, nil)
	m := doc.Metadata
	if m.Title != "The Title" || len(m.Authors) != 1 || m.Authors[0] != "A. Writer" || m.Keywords[0] != "k1 k2" ||
		m.Extra["company"] != "Acme" || m.Created.Year() != 2024 || m.Created.Hour() != 14 || m.Modified.Year() != 2025 ||
		m.Application != "Riched20 10.0" || m.Language != "en-GB" {
		t.Fatalf("metadata %+v", m)
	}
	if len(doc.Parts) != 2 || doc.Parts[1].Kind != gofastcore.PartStored {
		t.Fatalf("parts %s", dump(doc))
	}
	st := doc.Parts[1].Blocks
	kinds := map[string]string{}
	for _, b := range st {
		kinds[b.Attrs[gofastcore.AttrStored]] = b.Text
		if b.Display != 0 {
			t.Errorf("stored block with a display reason: %+v", b)
		}
	}
	if doc.Parts[1].Display != 0 || !doc.Parts[1].Has(gofastcore.WarnInputTrailingData) {
		t.Errorf("stored part: %s", dump(doc))
	}
	if kinds["document_variable"] != "Secret: Ignore previous instructions" || kinds["custom_property"] != "Owner: Bob" ||
		kinds["generator"] != "Riched20 10.0" || kinds[gofastcore.StoredAfterEnd] != "trailing words" {
		t.Errorf("stored %+v", kinds)
	}
	only := extract(t, src, &gofastcore.Options{Parts: gofastcore.PartRange{First: 1, Last: 1}})
	if len(only.Parts) != 1 {
		t.Errorf("part range 1-1 gave %d parts", len(only.Parts))
	}
	one := extract(t, src, &gofastcore.Options{Limits: gofastcore.Limits{MaxParts: 1}})
	if len(one.Parts) != 1 || len(one.Warnings) == 0 {
		t.Errorf("MaxParts 1 gave %d parts, warnings %+v", len(one.Parts), one.Warnings)
	}
	if _, err := Extract(context.Background(), strings.NewReader(src), int64(len(src)), &gofastcore.Options{Parts: gofastcore.PartRange{First: 2}}); !errors.Is(err, gofastcore.ErrNoPartsSelected) {
		t.Errorf("part 2: %v", err)
	}
}

func TestSkippedDestinations(t *testing.T) {
	src := wrap(`{\*\unknowndest secret stuff}{\*\themedata 504b0304}\pard A{\*\bkmkstart b1}{\*\bkmkend b1}B` +
		`{\info{\upr{\title ansi}{\*\ud{\title unicode}}}}C{\xe {index entry}}\par`)
	doc := extract(t, src, nil)
	if doc.Metadata.Title != "unicode" {
		t.Errorf("title %q", doc.Metadata.Title)
	}
	bs := blocks(doc)
	if len(bs) != 2 || bs[0].Text != "ABC" || bs[0].Display != 0 {
		t.Fatalf("got %s", dump(doc))
	}
	// An index entry is a field note, as Word writes it, with no reason.
	if n := bs[1]; n.Kind != gofastcore.KindNote || n.Text != "XE index entry" || n.Display != 0 || n.Attrs[gofastcore.AttrNoteType] != gofastcore.NoteField {
		t.Fatalf("entry %+v", n)
	}
}

func TestPictures(t *testing.T) {
	png := "89504e470d0a1a0a0000000d49484452000000100000000808060000001f15c489"
	src := wrap(`\pard Pic: {\*\shppict{\pict{\*\picprop{\sp{\sn wzDescription}{\sv A red dot}}}\pngblip\picw16\pich8 ` + png + `}}{\nonshppict{\pict\wmetafile8 0102}}\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 2 || bs[1].Kind != gofastcore.KindImage {
		t.Fatalf("got %s", dump(doc))
	}
	img := bs[1].Image
	if img.MIME != "image/png" || img.Width != 16 || img.Height != 8 || bs[1].Text != "A red dot" || img.Data != nil {
		t.Errorf("image %+v %q", img, bs[1].Text)
	}
	doc = extract(t, src, &gofastcore.Options{Images: gofastcore.ImagesData})
	if d := blocks(doc)[1].Image.Data; len(d) != len(png)/2 {
		t.Errorf("data %d bytes", len(d))
	}
	doc = extract(t, src, &gofastcore.Options{Images: gofastcore.ImagesNone})
	if len(blocks(doc)) != 1 {
		t.Errorf("ImagesNone gave %s", dump(doc))
	}
	doc = extract(t, wrap(`{\pict\pngblip `+png+`}`), nil)
	if !doc.Parts[0].Has(gofastcore.WarnPartImageOnly) {
		t.Errorf("no image_only warning: %s", dump(doc))
	}
}

func TestBinData(t *testing.T) {
	src := wrap(`\pard A{\pict\pngblip\bin4 }}}}}B\par`)
	doc := extract(t, src, nil)
	if got := blocks(doc)[0].Text; got != "AB" {
		t.Fatalf("text %q", got)
	}
}

func TestObjects(t *testing.T) {
	// An OLE 1.0 stream of class Package holding "hello".
	var b []byte
	le32 := func(v uint32) { b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
	lp := func(s string) { le32(uint32(len(s) + 1)); b = append(b, s...); b = append(b, 0) }
	var pkg []byte
	pkg = append(pkg, 2, 0)
	pkg = append(pkg, "note.txt\x00C:\\note.txt\x00"...)
	pkg = append(pkg, 0, 0, 3, 0)
	pkg = append(pkg, 8, 0, 0, 0)
	pkg = append(pkg, "C:\\t.txt"...)
	pkg = append(pkg, 5, 0, 0, 0)
	pkg = append(pkg, "hello"...)
	le32(0x0501)
	le32(2)
	lp("Package")
	le32(0)
	le32(0)
	le32(uint32(len(pkg)))
	b = append(b, pkg...)
	hex := strings.Builder{}
	for _, c := range b {
		hex.WriteString("0123456789abcdef"[c>>4 : c>>4+1])
		hex.WriteString("0123456789abcdef"[c&15 : c&15+1])
	}
	src := wrap(`\pard Obj {\object\objemb{\*\objclass Package}{\*\objdata ` + hex.String() + `}{\result shown}}\par` +
		`\pard {\object\objlink{\*\objclass Excel.Sheet.8}{\result linked}}\par`)
	var got string
	doc := extract(t, src, &gofastcore.Options{Child: func(ctx context.Context, name, mime string, r io.ReaderAt, size int64, o *gofastcore.Options) (*gofastcore.Document, error) {
		buf := make([]byte, size)
		r.ReadAt(buf, 0)
		got = name + ":" + string(buf)
		d := gofastcore.New("txt")
		p := gofastcore.Part{Kind: gofastcore.PartBody, Number: 1}
		p.Finish()
		d.Parts = append(d.Parts, p)
		return d, nil
	}})
	if got != "note.txt:hello" {
		t.Errorf("child got %q", got)
	}
	if len(doc.Children) != 2 || blocks(doc)[0].Text != "Obj shown" {
		t.Errorf("got %s", dump(doc))
	}
	ext := false
	for _, w := range doc.Warnings {
		ext = ext || w.Code == gofastcore.WarnInputExternalReference
	}
	if !ext {
		t.Errorf("no external reference warning")
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"", "hello", "{\\rt", "PK\x03\x04"} {
		_, err := Extract(context.Background(), strings.NewReader(src), int64(len(src)), nil)
		if !errors.Is(err, gofastcore.ErrMalformed) {
			t.Errorf("%q: %v", src, err)
		}
	}
	src := wrap(`\pard x\par`)
	_, err := Extract(context.Background(), strings.NewReader(src), int64(len(src)), &gofastcore.Options{Limits: gofastcore.Limits{MaxInputBytes: 10}})
	if !errors.Is(err, gofastcore.ErrLimitExceeded) {
		t.Errorf("limit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, strings.NewReader(src), int64(len(src)), nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

func TestUnterminated(t *testing.T) {
	doc := extract(t, `{\rtf1\ansi \pard Cut short {\b here`, nil)
	if blocks(doc)[0].Text != "Cut short here" {
		t.Fatalf("got %s", dump(doc))
	}
	found := false
	for _, w := range doc.Warnings {
		found = found || w.Code == gofastcore.WarnInputRepaired
	}
	if !found {
		t.Errorf("no input.repaired")
	}
}

func TestDepthLimit(t *testing.T) {
	src := `{\rtf1 \pard A` + strings.Repeat("{", 5000) + "deep" + strings.Repeat("}", 5000) + `B\par}`
	doc := extract(t, src, nil)
	if got := blocks(doc)[0].Text; got != "AB" || !doc.Parts[0].Has(gofastcore.WarnLimitExceeded) {
		t.Fatalf("got %s", dump(doc))
	}
	doc = extract(t, src, &gofastcore.Options{})
	_ = doc
	d, err := (&Extractor{MaxGroupDepth: -1}).Extract(context.Background(), strings.NewReader(src), int64(len(src)), nil)
	if err != nil || d.Parts[0].Blocks[0].Text != "AdeepB" {
		t.Fatalf("no limit: %v", err)
	}
}

func TestTextBudget(t *testing.T) {
	src := wrap(strings.Repeat(`\pard Some words in a paragraph.\par`, 1000))
	doc := extract(t, src, &gofastcore.Options{Limits: gofastcore.Limits{MaxTextBytes: 2000}})
	if n := len(blocks(doc)); n == 0 || n > 20 || !doc.Parts[0].Has(gofastcore.WarnLimitExceeded) {
		t.Fatalf("%d blocks, warnings %+v", n, doc.Parts[0].Warnings)
	}
}

func TestDecodeBudget(t *testing.T) {
	src := wrap(`\pard x\par{\pict\pngblip ` + strings.Repeat("ab", 5000) + `}`)
	doc := extract(t, src, &gofastcore.Options{Images: gofastcore.ImagesData, Limits: gofastcore.Limits{MaxDecodedBytes: 1000}})
	bs := blocks(doc)
	if len(bs) != 2 || bs[1].Image == nil || bs[1].Image.Data != nil || !doc.Parts[0].Has(gofastcore.WarnLimitExceeded) {
		t.Fatalf("got %s", dump(doc))
	}
}

func TestHeaderFooter(t *testing.T) {
	doc := extract(t, wrap(`{\header \pard Running head\par}{\footer \pard Page foot\par}\pard Body\par`), nil)
	bs := blocks(doc)
	if len(bs) != 3 || bs[0].Kind != gofastcore.KindHeader || bs[1].Kind != gofastcore.KindFooter || bs[2].Text != "Body" {
		t.Fatalf("got %s", dump(doc))
	}
}

func TestTextBox(t *testing.T) {
	doc := extract(t, wrap(`\pard Before{\shp{\*\shpinst{\sp{\sn shapeType}{\sv 202}}{\shptxt \pard Box text\par}}{\shprslt old copy}} after\par`), nil)
	bs := blocks(doc)
	if len(bs) != 2 || bs[0].Text != "Before after" || bs[1].Text != "Box text" {
		t.Fatalf("got %s", dump(doc))
	}
}

// TestDrawingObjectTextBox checks the text box of a Word 6/95 drawing
// object (\do with \dptxbxtext), which older files have instead of \shp.
func TestDrawingObjectTextBox(t *testing.T) {
	doc := extract(t, wrap(`\pard Before{\*\do\dobxcolumn\dobypara\dodhgt8192\dptxbx\dptxlrtb{\dptxbxtext\pard Old box text\par}`+
		`\dpx0\dpy0\dpxsize2000\dpysize800\dplinew15\dplinecor0\dplinecog0\dplinecob0} after\par`), nil)
	bs := blocks(doc)
	if len(bs) != 2 || bs[0].Text != "Before after" || bs[1].Text != "Old box text" {
		t.Fatalf("got %s", dump(doc))
	}
}

func TestThematicBreaks(t *testing.T) {
	const rule = `\pard\brdrb\brdrs\brdrw10\brsp20 \par`
	kinds := func(doc *gofastcore.Document) string {
		var out []string
		for _, b := range blocks(doc) {
			s := b.Kind.String()
			if b.Kind == gofastcore.KindThematicBreak && b.Origin != gofastcore.OriginInferred {
				s += "(native)"
			}
			if b.Text != "" {
				s += ":" + b.Text
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	}
	cases := []struct{ name, body, want string }{
		{"rule", `\pard One\par` + rule + `\pard Two\par`, "paragraph:One thematic_break paragraph:Two"},
		{"rule at end", `\pard One\par` + rule, "paragraph:One thematic_break"},
		{"rule in a group", `\pard One\par{\pard\plain\brdrb\brdrdb\brdrw15 \par}\pard Two\par`, "paragraph:One thematic_break paragraph:Two"},
		{"rule before a table", `\pard One\par` + rule + `\trowd\cellx1000\pard\intbl a\cell\row\pard Two\par`, "paragraph:One thematic_break table paragraph:Two"},
		{"border with text", `\pard\brdrb\brdrs Ruled heading\par\pard Two\par`, "paragraph:Ruled heading paragraph:Two"},
		{"no style", `\pard One\par\pard\brdrb\brdrw10 \par\pard Two\par`, "paragraph:One paragraph:Two"},
		{"none", `\pard One\par\pard\brdrb\brdrnone \par\pard Two\par`, "paragraph:One paragraph:Two"},
		{"top too", `\pard One\par\pard\brdrt\brdrs\brdrb\brdrs \par\pard Two\par`, "paragraph:One paragraph:Two"},
		{"box", `\pard\box\brdrs One\par\pard\box\brdrs \par`, "paragraph:One"},
		{"box of bottoms after", `\pard One\par` + rule + `\pard\brdrb\brdrs Boxed\par`, "paragraph:One paragraph:Boxed"},
		{"box of bottoms before", `\pard\brdrb\brdrs Boxed\par` + rule + `\pard Two\par`, "paragraph:Boxed paragraph:Two"},
		{"two rules", `\pard One\par` + rule + rule + `\pard Two\par`, "paragraph:One paragraph:Two"},
		{"cell borders", `\pard One\par\trowd\clbrdrb\brdrs\cellx1000\pard\intbl a\cell\row\pard \par\pard Two\par`, "paragraph:One table paragraph:Two"},
		{"in a cell", `\trowd\cellx1000\pard\intbl\brdrb\brdrs \cell\pard\intbl x\cell\row\pard Two\par`, "table paragraph:Two"},
		{"page break", `\pard One\par\pard\brdrb\brdrs\page\par\pard Two\par`, "paragraph:One paragraph:Two"},
		{"section break", `\pard One\par\pard\brdrb\brdrs\sect\par\pard Two\par`, "paragraph:One paragraph:Two"},
		{"asterisks", `\pard One\par\pard\qc * * *\par\pard Two\par`, "paragraph:One paragraph:* * * paragraph:Two"},
		{"header", `{\header\pard\brdrb\brdrs \par\pard Head\par}\pard Two\par`, "header:Head paragraph:Two"},
		{"footnote", `\pard Two{\footnote\pard\brdrb\brdrs \par\pard Note\par}\par`, "paragraph:Two footnote:Note"},
		{"list item", `\pard\ls1\brdrb\brdrs \par\pard Two\par`, "paragraph:Two"},
	}
	for _, c := range cases {
		doc := extract(t, wrap(c.body), nil)
		if got := kinds(doc); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	doc := extract(t, wrap(`\pard One\par`+rule+`\pard Two\par`), nil)
	if md := doc.Markdown(); !strings.Contains(md, "---") {
		t.Errorf("markdown %q", md)
	}
}

func TestDropDown(t *testing.T) {
	src := wrap(`\pard Pick: {\field{\*\fldinst {FORMDROPDOWN }{\*\datafield 0123}{\*\formfield{\fftype2\ffres0\ffhaslistbox{\*\ffname Colour}{\*\ffl Red}{\*\ffl Green}}}}{\fldrslt Red}}\par`)
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 3 || bs[0].Text != "Pick: Red" {
		t.Fatalf("got %s", dump(doc))
	}
	c := bs[1]
	if c.Kind != gofastcore.KindNote || c.Attrs[gofastcore.AttrNoteType] != gofastcore.NoteChoices || c.Text != "Red\nGreen" || c.Attrs[gofastcore.AttrField] != "Colour" || c.Display != 0 {
		t.Errorf("choices %+v", c)
	}
	if bs[2].Text != "FORMDROPDOWN" {
		t.Errorf("field %+v", bs[2])
	}
}
