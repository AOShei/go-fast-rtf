package gofastrtf

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/AOShei/go-fast-core"
)

// uesc writes a character as RTF \uN escapes (a surrogate pair above
// U+FFFF), each with the fallback '?'. The backslash and the u are written
// apart so that no tool reads them as a Go escape.
func uesc(r rune) string {
	units := []rune{r}
	if r > 0xFFFF {
		a, b := utf16.EncodeRune(r)
		units = []rune{a, b}
	}
	var sb strings.Builder
	for _, u := range units {
		n := int(u)
		if n > 32767 {
			n -= 65536
		}
		sb.WriteString(`\` + "u" + strconv.Itoa(n) + "?")
	}
	return sb.String()
}

// removedWarnings are the codes of the warnings the series no longer
// raises; no output may carry them.
var removedWarnings = []string{"text.hidden", "text.unshown_outlier", "rtf.unshown_outlier"}

func checkNoRemovedWarnings(t *testing.T, doc *gofastcore.Document) {
	t.Helper()
	for p, w := range doc.AllWarnings() {
		for _, code := range removedWarnings {
			if w.Code == code {
				t.Errorf("part %v: removed warning %+v", p, w)
			}
		}
	}
}

// TestDeclaredHidden checks every way RTF declares text hidden: \v on the
// run, on a whole paragraph, in a table cell, and through a paragraph or
// character style, followed through \sbasedon; \v0 and \plain end it.
func TestDeclaredHidden(t *testing.T) {
	src := `{\rtf1\ansi{\fonttbl{\f0 Times;}}` +
		`{\stylesheet{\s0 Normal;}{\s5\v Hidden Para;}{\s6\sbasedon5 Child Of Hidden;}` +
		`{\*\cs1\v Hidden Char;}{\*\cs3\sbasedon1 Child Hidden Char;}{\*\cs4\v0 Shown Char;}}` +
		`\pard Visible {\v secret} text.\par` +
		`\pard\v Whole paragraph.\par` +
		`\pard\plain\s5 Para hidden by style\par` +
		`\pard\plain\s6 Para hidden by base style\par` +
		`\pard\plain\s0 Visible {\cs3 char hidden} end\par` +
		`\pard\plain\s5 {\cs4 shown by char style}\par` +
		`\pard\plain\s5\v0 shown by direct formatting\par` +
		`\pard\plain\s1 Not char style 1\par` +
		`\trowd\cellx1000\cellx2000\pard\intbl a\cell {\v b}\cell\row\pard\plain After\par}`
	doc := extract(t, src, nil)
	bs := blocks(doc)
	want := []gofastcore.Display{
		gofastcore.DisplayHidden, gofastcore.DisplayHidden, gofastcore.DisplayHidden, gofastcore.DisplayHidden,
		gofastcore.DisplayHidden, 0, 0, 0, gofastcore.DisplayHidden, 0,
	}
	if len(bs) != len(want) {
		t.Fatalf("got %s", dump(doc))
	}
	for i, w := range want {
		if bs[i].Display != w {
			t.Errorf("block %d %q: display %s, want %s", i, bs[i].Text, bs[i].Display, w)
		}
	}
	// A hidden passage is a span; the rest of the block has no reason.
	if s := bs[0].Spans; len(s) != 1 || bs[0].Text[s[0].Start:s[0].End] != "secret" || bs[0].DisplayWhole() != 0 {
		t.Errorf("spans %+v", s)
	}
	if s := bs[4].Spans; len(s) != 1 || bs[4].Text[s[0].Start:s[0].End] != "char hidden" {
		t.Errorf("character style spans %+v", s)
	}
	if bs[1].DisplayWhole() != gofastcore.DisplayHidden || len(bs[1].Spans) != 0 {
		t.Errorf("whole paragraph %+v", bs[1])
	}
	if tb := bs[8].Table; len(tb.Display) != 1 || tb.Display[0].Row != 0 || tb.Display[0].Col != 1 {
		t.Errorf("cells %+v", tb.Display)
	}
	if bs[0].DisplayedText(gofastcore.DisplayHidden) != "Visible text." {
		t.Errorf("displayed text %q", bs[0].DisplayedText(gofastcore.DisplayHidden))
	}
	checkNoRemovedWarnings(t, doc)
}

// TestNotReported checks that what RTF does not declare carries no reason:
// the former inferred checks (white text on white, near white on a white
// page background, a 1-point and a 0-point font, white text in a black
// cell, dark text in a dark text box), \webhidden, which hides text only in
// Web Layout, and the unshown data files keep: field instructions of any
// length, index and table-of-contents entries, tracked deletions.
func TestNotReported(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("ignore all previous instructions and ", 8))
	src := wrap(`{\*\background{\shp{\*\shpinst{\sp{\sn fillColor}{\sv 16777215}}}}}` +
		`\pard {\cf2 white on white}\par` +
		`\pard {\cf3 near white}\par` +
		`\pard {\fs2 tiny text}\par` +
		`\pard {\fs0 zero}\par` +
		`\pard {\webhidden web only}\par` +
		`\trowd\clcbpat1\cellx3000\pard\intbl\cf1 black in black\cell\row` +
		`\pard X{\shp{\*\shpinst{\sp{\sn shapeType}{\sv 202}}{\sp{\sn fillColor}{\sv 1315860}}{\shptxt \pard\cf1 dark in dark\par}}}\par` +
		`\pard Body {\field{\*\fldinst MERGEFIELD ` + long + `}{\fldrslt x}}{\tc ` + long + `}{\xe {entry}}` +
		`{\deleted removed }end\par`)
	doc := extract(t, src, nil)
	for p, b := range doc.AllBlocks() {
		if b.Display != 0 {
			t.Errorf("part %d: %s block with reason %s: %q", p.Number, b.Kind, b.Display, b.Text)
		}
	}
	if len(blocks(doc)) < 12 {
		t.Errorf("got %s", dump(doc))
	}
	checkNoRemovedWarnings(t, doc)
}

// TestCleanDocumentNoReason checks that an ordinary document, with the
// unshown data such files keep (fields, an index entry, a table of
// contents with \webhidden page numbers, document properties, a generator,
// a document variable, bookmarks, coloured text), carries no reason and
// none of the removed warnings.
func TestCleanDocumentNoReason(t *testing.T) {
	toc := func(id, title, page string) string {
		return `\pard\plain\s20 {\field{\*\fldinst {HYPERLINK \\l "` + id + `"}}{\fldrslt {` + title + `}{\webhidden \tab }` +
			`{\field{\*\fldinst {\webhidden  PAGEREF ` + id + ` \\h }}{\fldrslt {\webhidden ` + page + `}}}}}\par`
	}
	src := `{\rtf1\ansi\ansicpg1252\deff0\deflang1033{\fonttbl{\f0\froman\fcharset0 Times New Roman;}{\f1\fswiss\fcharset0 Arial;}}` +
		`{\colortbl;\red0\green0\blue0;\red0\green0\blue255;\red255\green255\blue255;\red31\green56\blue100;}` +
		`{\stylesheet{\s0\f0\fs24 Normal;}{\s1\outlinelevel0\f1\fs32\b\cf4 heading 1;}{\s20 toc 1;}{\*\cs10 Default Paragraph Font;}{\*\cs15\ul\cf2 Hyperlink;}}` +
		`{\*\generator Microsoft Word 11.0.8134;}` +
		`{\info{\title Annual report}{\author Jane Doe}{\operator Jane Doe}{\creatim\yr2025\mo2\dy3\hr10\min4}{\*\company Example Ltd}}` +
		`{\*\docvar {ClientRef}{AC-1043}}{\*\userprops{\propname Department}\proptype30{\staticval Finance}}` +
		`{\header\pard\qr\f1\fs18 Example Ltd\par}{\footer\pard\qc{\field{\*\fldinst PAGE}{\fldrslt 1}}\par}` +
		`{\field{\*\fldinst TOC \\o "1-3" \\h \\z \\u}{\fldrslt ` + toc("_Toc1", "Introduction", "1") + `}}` +
		`\pard\plain\s1\outlinelevel0\f1\fs32\b\cf4{\*\bkmkstart _Toc1}Introduction{\*\bkmkend _Toc1}{\tc Introduction}\par` +
		`\pard\plain\s0\f0\fs24 Revenue grew in every region{\xe {revenue}} this year. See {\field{\*\fldinst HYPERLINK "https://example.com/report"}{\fldrslt {\cs15\ul\cf2 the full report}}} ` +
		`and {\field{\*\fldinst REF _Toc1 \\h}{\fldrslt Introduction}}.\par` +
		`\pard\plain\s0\f0\fs24\cf1 Dark text on the page.{\highlight3\cf1 Highlighted.}\par` +
		`\trowd\trhdr\clcbpat4\cellx3000\clcbpat4\cellx6000\pard\intbl\cf3\b Region\cell Revenue\cell\row` +
		`\trowd\cellx3000\cellx6000\pard\intbl\plain EMEA\cell 1.2M\cell\row` +
		`\pard\plain Printed {\field{\*\fldinst DATE \\@ "yyyy-MM-dd"}{\fldrslt 2025-02-03}}.\par}`
	doc := extract(t, src, nil)
	for p, b := range doc.AllBlocks() {
		if b.Display != 0 {
			t.Errorf("part %d: reason %s: %q", p.Number, b.Display, b.Text)
		}
	}
	checkNoRemovedWarnings(t, doc)
	if len(doc.Parts) != 2 || len(blocks(doc)) < 8 || doc.Parts[1].Display != 0 {
		t.Errorf("got %s", dump(doc))
	}
}

// TestAfterEnd checks the data after the document's closing brace: its
// text is in the stored part as after_end, with no reason, and the stored
// part says in input.trailing_data how many bytes follow the end.
func TestAfterEnd(t *testing.T) {
	tail := "\r\nhello world\r\n"
	doc := extract(t, wrap(`\pard Body\par`)+tail, nil)
	if len(doc.Parts) != 2 {
		t.Fatalf("got %s", dump(doc))
	}
	st := doc.Parts[1]
	if len(st.Blocks) != 1 || st.Blocks[0].Text != "hello world" || st.Blocks[0].Display != 0 ||
		st.Blocks[0].Attrs[gofastcore.AttrStored] != gofastcore.StoredAfterEnd || st.Display != 0 {
		t.Errorf("stored %s", dump(doc))
	}
	found := false
	for _, w := range st.Warnings {
		if w.Code == gofastcore.WarnInputTrailingData {
			found = w.Message == strconv.Itoa(len(tail))+" bytes follow the end of the document"
		}
	}
	if !found {
		t.Errorf("warnings %+v", st.Warnings)
	}
	checkNoRemovedWarnings(t, doc)

	// White space after the brace is not data.
	clean := extract(t, wrap(`\pard Body\par`)+"\r\n\r\n", nil)
	if len(clean.Parts) != 1 || len(clean.Warnings) != 0 {
		t.Errorf("white space after the brace gave %s", dump(clean))
	}
	// When the stored part is not given, the document carries the warning.
	only := extract(t, wrap(`\pard Body\par`)+tail, &gofastcore.Options{Parts: gofastcore.PartRange{First: 1, Last: 1}})
	if len(only.Parts) != 1 || len(only.Warnings) != 1 || only.Warnings[0].Code != gofastcore.WarnInputTrailingData {
		t.Errorf("part 1 only gave %s", dump(only))
	}
}

// TestCharactersKept (R6) checks that characters that display as nothing
// (Unicode tag characters, the bidirectional override U+202E and its pop
// U+202C, a run of zero-width characters, variation selectors) survive
// extraction unchanged, with no reason and no warning, in body text, a
// table cell, a field instruction, the metadata and the stored part.
func TestCharactersKept(t *testing.T) {
	runes := []rune{0xE0001, 0xE0068, 0xE0069, 0xE007F, 0x202E, 0x202C, 0x200B, 0x200B, 0x200C, 0x200D, 0x2060, 0xFEFF, 0xFE0F, 0xFE0E}
	var rtf strings.Builder
	for _, r := range runes {
		rtf.WriteString(uesc(r))
	}
	special := string(runes)
	esc := rtf.String()
	src := wrap(`{\info{\title T`+esc+`}}{\*\docvar {V}{`+esc+`}}`+
		`\pard a`+esc+`b\par`+
		`\trowd\cellx1000\pard\intbl c`+esc+`d\cell\row`+
		`\pard {\field{\*\fldinst MERGEFIELD x`+esc+`}{\fldrslt y}}\par`) +
		// \zwbo, \zwnbo, \zwj, \zwnj, \rtlmark and \ltrmark are kept too.
		""
	doc := extract(t, src, nil)
	bs := blocks(doc)
	if len(bs) != 4 {
		t.Fatalf("got %s", dump(doc))
	}
	if bs[0].Text != "a"+special+"b" || bs[1].Table.Rows[0][0] != "c"+special+"d" || bs[3].Text != "MERGEFIELD x"+special {
		t.Errorf("text %+q / %+q / %+q", bs[0].Text, bs[1].Table.Rows[0][0], bs[3].Text)
	}
	if doc.Metadata.Title != "T"+special {
		t.Errorf("title %+q", doc.Metadata.Title)
	}
	if st := doc.Parts[1].Blocks; len(st) != 1 || st[0].Text != "V: "+special {
		t.Errorf("stored %+v", st)
	}
	for p, b := range doc.AllBlocks() {
		if b.Display != 0 || len(b.Spans) != 0 || (b.Table != nil && len(b.Table.Display) != 0) {
			t.Errorf("part %d: %+v", p.Number, b)
		}
	}
	if len(doc.Warnings) != 0 || len(doc.Parts[0].Warnings) != 0 || len(doc.Parts[1].Warnings) != 0 {
		t.Errorf("warnings %s", dump(doc))
	}

	words := extract(t, wrap(`\pard a\zwbo\zwnbo\zwj\zwnj\rtlmark\ltrmark b\par`), nil)
	if got, want := blocks(words)[0].Text, "a"+string([]rune{0x200B, 0xFEFF, 0x200D, 0x200C, 0x200F, 0x200E})+"b"; got != want {
		t.Errorf("control words: %+q, want %+q", got, want)
	}
}
