package reader

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AOShei/go-fast-core"
)

// flowKind says where the blocks of a flow go.
type flowKind uint8

const (
	flowBody     flowKind = iota // the body part
	flowHeader                   // running headers, in the body part
	flowFooter                   // running footers, in the body part
	flowBox                      // a text box: after the paragraph it is in
	flowFootnote                 // one footnote block
	flowNote                     // one comment note
)

// run is a range of the paragraph's text with the same display reasons,
// style and link. Text with none of them has no run.
type run struct {
	start, end int
	hidden     gofastcore.Display
	style      gofastcore.Style
	href       string
}

// flow builds the paragraphs and tables of one stream of text: the body, a
// header, a footnote, a text box.
type flow struct {
	r      *reader
	kind   flowKind
	parent *flow
	shape  *shape // the shape a text box belongs to, if any

	text      []byte
	runs      []run
	anchor    string
	marker    *strbuf
	pnLevel   int32
	pnOrdered int8 // -1 bullet, 1 ordered, 0 not said
	lastP     para

	del       []byte
	delFull   bool  // the deletion reached maxDeletion; nothing more is kept
	delLost   int64 // bytes of deleted text past maxDeletion
	delAuthor string
	delDate   string

	after  []gofastcore.Block
	tables []*table
	defs   []rowDef

	collected []gofastcore.Block // footnotes and comments
	autoMark  bool               // a footnote with an automatic number
	endnote   bool
	author    string
	date      string
}

func appendRune(b []byte, c rune) []byte {
	if s := gofastcore.Ligature(c); s != "" {
		return append(b, s...)
	}
	return utf8.AppendRune(b, c)
}

// add appends one character with the properties of group g.
func (f *flow) add(c rune, g *group) {
	r := f.r
	f.lastP = g.p
	if g.c.deleted {
		if len(f.del) == 0 {
			f.delAuthor, f.delDate = r.revAuthor(g.c.revDel), dttm(g.c.dttmDel)
		}
		// As strbuf.addRune: the text is cut at the first rune that does
		// not fit.
		if n := runeBytes(c); f.delFull || len(f.del)+n > maxDeletion {
			f.delFull = true
			f.delLost += int64(n)
		} else {
			f.del = appendRune(f.del, c)
		}
		return
	}
	if len(f.del) > 0 {
		f.flushDel()
	}
	h := displayOf(&g.c)
	var st gofastcore.Style
	href := ""
	if r.opts.Spans {
		st, href = styleOf(&g.c), g.href
	}
	start := len(f.text)
	f.text = appendRune(f.text, c)
	if int64(len(f.text)) > r.cfg.Text.Left() {
		r.textCut = true
	}
	if h == 0 && st == 0 && href == "" {
		return
	}
	if n := len(f.runs); n > 0 {
		last := &f.runs[n-1]
		if last.end == start && last.hidden == h && last.style == st && last.href == href {
			last.end = len(f.text)
			return
		}
	}
	f.runs = append(f.runs, run{start: start, end: len(f.text), hidden: h, style: st, href: href})
}

func styleOf(c *chr) gofastcore.Style {
	var s gofastcore.Style
	if c.bold {
		s |= gofastcore.StyleBold
	}
	if c.italic {
		s |= gofastcore.StyleItalic
	}
	if c.ul {
		s |= gofastcore.StyleUnderline
	}
	if c.strike {
		s |= gofastcore.StyleStrike
	}
	if c.sup {
		s |= gofastcore.StyleSuper
	}
	if c.sub {
		s |= gofastcore.StyleSub
	}
	return s
}

// flushDel keeps the text removed by a tracked change as a note.
func (f *flow) flushDel() {
	if len(f.del) == 0 {
		return
	}
	text := cleanText(string(f.del))
	lost := f.delLost
	f.del, f.delLost, f.delFull = f.del[:0], 0, false
	if text == "" {
		return
	}
	if lost > 0 {
		f.r.cut(atBody, "tracked deletions are cut at", maxDeletion, 1, lost)
	}
	f.r.deleted++
	attrs := map[string]string{gofastcore.AttrNoteType: gofastcore.NoteDeletion}
	if f.delAuthor != "" {
		attrs[gofastcore.AttrAuthor] = f.delAuthor
	}
	if f.delDate != "" {
		attrs[gofastcore.AttrDate] = f.delDate
	}
	b := gofastcore.Block{Kind: gofastcore.KindNote, Text: text, Attrs: attrs}
	f.r.holdBlock(&b)
	f.after = append(f.after, b)
}

// maxDeletion bounds the text of one tracked deletion.
const maxDeletion = 1 << 20

// tableLevel is the nesting depth of tables a paragraph is in; 0 for none.
func tableLevel(p para) int {
	switch {
	case p.itap > 0:
		return int(min(p.itap, 64))
	case p.intbl:
		return 1
	}
	return 0
}

// endPara ends the paragraph being read, with properties p.
func (f *flow) endPara(p para) { f.endParaAt(p, tableLevel(p)) }

func (f *flow) endParaAt(p para, level int) {
	f.flushDel()
	text, spans, hidden := settle(f.text, f.runs)
	marker := ""
	hasMarker := f.marker != nil
	if hasMarker {
		marker = cleanMarker(f.marker.String())
		f.r.cutBuf(atBody, "list markers (\\listtext, \\pntext) are cut at", f.marker)
	}
	if level > 0 {
		f.ensureTables(level)
		f.closeDeeper(level)
		t := f.tables[level-1]
		if marker != "" && text != "" {
			text = marker + " " + text
		} else if text == "" {
			text = marker
		}
		t.c.addPara(text, hidden, spans)
		t.after = append(t.after, f.after...)
	} else {
		f.finishTables()
		if text == "" && marker != "" {
			text, marker = marker, ""
		}
		if text != "" {
			f.emit(f.r.paraBlock(f, p, text, spans, hidden, marker, hasMarker))
		}
		for _, b := range f.after {
			f.emit(b)
		}
		if f.kind == flowBody {
			f.r.held = 0 // all that was held has been added
		}
	}
	f.after = f.after[:0]
	f.text, f.runs, f.anchor, f.marker = f.text[:0], f.runs[:0], "", nil
	f.pnLevel, f.pnOrdered = 0, 0
}

// paraBlock makes the block of a paragraph outside a table.
func (r *reader) paraBlock(f *flow, p para, text string, spans []gofastcore.Span, hidden gofastcore.Display, marker string, hasMarker bool) gofastcore.Block {
	b := gofastcore.Block{Kind: gofastcore.KindParagraph, Text: text, Spans: spans, Display: hidden}
	b.Loc.Anchor = f.anchor
	if lvl := r.headingLevel(p); lvl > 0 {
		b.Kind, b.Marker = gofastcore.KindHeading, marker
		if lvl > 6 {
			b.Attrs = map[string]string{gofastcore.AttrLevel: strconv.Itoa(lvl)}
			lvl = 6
			r.clamped++
		}
		b.Level = uint8(lvl)
		return b
	}
	if hasMarker || p.ls > 0 {
		b.Kind, b.Marker = gofastcore.KindListItem, marker
		level := int32(1)
		switch {
		case p.ls > 0:
			level = min(max(p.ilvl, 0), 8) + 1
		case f.pnLevel > 0:
			level = min(f.pnLevel, 9)
		}
		b.Level = uint8(level)
		b.Ordered = r.ordered(p, f, marker)
	}
	return b
}

// ordered says whether a list item is numbered: from the list table, from
// the old-style \pn settings, or from its marker.
func (r *reader) ordered(p para, f *flow, marker string) bool {
	if p.ls > 0 {
		if id, ok := r.lsList[p.ls]; ok {
			if levels := r.lists[id]; int(p.ilvl) < len(levels) && p.ilvl >= 0 {
				nfc := levels[p.ilvl]
				return nfc != 23 && nfc != 255
			}
		}
	}
	if f.pnOrdered != 0 {
		return f.pnOrdered > 0
	}
	if marker == "" {
		return false
	}
	c, _ := utf8.DecodeRuneInString(marker)
	return unicode.IsDigit(c) || unicode.IsLetter(c) || c == '('
}

// headingLevel is the outline level of a paragraph, from 1, or 0 for
// body text.
func (r *reader) headingLevel(p para) int {
	if p.outline >= 0 && p.outline < 9 {
		return int(p.outline) + 1
	}
	s := p.style
	for range 16 {
		st, ok := r.styles[s]
		if !ok || st.char {
			return 0
		}
		if st.outline >= 0 && st.outline < 9 {
			return int(st.outline) + 1
		}
		if n := headingName(st.name.String()); n > 0 {
			return n
		}
		if st.basedOn < 0 || st.basedOn == s {
			return 0
		}
		s = st.basedOn
	}
	return 0
}

// headingName returns N for a style named "heading N".
func headingName(name string) int {
	name = strings.ToLower(strings.TrimSpace(strings.TrimSpace(name)))
	name = strings.TrimSpace(strings.TrimSuffix(name, ";"))
	rest, ok := strings.CutPrefix(name, "heading ")
	if !ok || len(rest) != 1 || rest[0] < '1' || rest[0] > '9' {
		return 0
	}
	return int(rest[0] - '0')
}

// emit hands a finished block to where the flow's blocks go.
func (f *flow) emit(b gofastcore.Block) {
	switch f.kind {
	case flowHeader, flowFooter:
		switch b.Kind {
		case gofastcore.KindParagraph, gofastcore.KindHeading, gofastcore.KindListItem:
			b.Kind = gofastcore.KindHeader
			if f.kind == flowFooter {
				b.Kind = gofastcore.KindFooter
			}
			b.Level, b.Ordered, b.Attrs = 0, false, nil
			if b.Marker != "" {
				b.Text = b.Marker + " " + b.Text
				b.Spans = shiftSpans(b.Spans, len(b.Marker)+1)
				b.Marker = ""
			}
		}
		f.r.add(b)
	case flowBox:
		if f.shape != nil {
			// Held until the shape ends: its properties, fHidden among
			// them, may follow its text.
			f.r.holdBlock(&b)
			f.shape.boxed = append(f.shape.boxed, boxed{parent: f.parent, b: b})
		} else if f.parent != nil {
			f.r.holdBlock(&b)
			f.parent.after = append(f.parent.after, b)
		} else {
			f.r.add(b)
		}
	case flowFootnote, flowNote:
		f.r.holdBlock(&b)
		f.collected = append(f.collected, b)
	default:
		f.r.add(b)
	}
}

func shiftSpans(spans []gofastcore.Span, by int) []gofastcore.Span {
	for i := range spans {
		spans[i].Start += by
		spans[i].End += by
	}
	return spans
}

// end finishes the flow: the paragraph left open and the tables.
func (f *flow) end() {
	if len(f.text) > 0 || f.marker != nil || len(f.del) > 0 {
		f.endPara(f.lastP)
	}
	f.finishTables()
	for _, b := range f.after {
		f.emit(b)
	}
	f.after = f.after[:0]
}

// merge ends a footnote or comment flow and returns its text as one block,
// paragraphs separated by line breaks, and what else it held (tables,
// pictures, notes), which follows it.
func (f *flow) merge() (text string, spans []gofastcore.Span, hidden gofastcore.Display, rest []gofastcore.Block) {
	f.end()
	var buf []byte
	var runs []run
	for _, b := range f.collected {
		switch b.Kind {
		case gofastcore.KindParagraph, gofastcore.KindHeading, gofastcore.KindListItem:
		default:
			rest = append(rest, b)
			continue
		}
		t := b.Text
		if b.Marker != "" {
			t = b.Marker + " " + t
		}
		if len(buf) > 0 {
			buf = append(buf, '\n')
		}
		at := len(buf)
		if b.Marker != "" {
			at += len(b.Marker) + 1
		}
		buf = append(buf, t...)
		if whole := b.DisplayWhole(); whole != 0 {
			runs = append(runs, run{start: len(buf) - len(t), end: len(buf), hidden: whole})
		}
		for _, s := range b.Spans {
			runs = append(runs, run{start: s.Start + at, end: s.End + at, hidden: s.Display, style: s.Style, href: s.Href})
		}
	}
	text, spans, hidden = settle(buf, runs)
	return text, spans, hidden, rest
}

// settle trims the paragraph's text and turns its runs into spans. A
// hidden reason that covers all of the text becomes a reason of the whole
// block, which is how the model says it; one that covers part stays on
// its spans.
func settle(raw []byte, runs []run) (string, []gofastcore.Span, gofastcore.Display) {
	from := len(raw) - len(bytes.TrimLeftFunc(raw, unicode.IsSpace))
	to := len(bytes.TrimRightFunc(raw, unicode.IsSpace))
	if from >= to {
		return "", nil, 0
	}
	text := raw[from:to]
	if len(runs) == 0 {
		return string(text), nil, 0
	}
	var all gofastcore.Display
	spans := make([]gofastcore.Span, 0, len(runs))
	for _, ru := range runs {
		s := gofastcore.Span{Start: max(ru.start, from) - from, End: min(ru.end, to) - from, Display: ru.hidden, Style: ru.style, Href: ru.href}
		if s.Start < s.End {
			spans = append(spans, s)
			all |= s.Display
		}
	}
	whole := all
	for bit := gofastcore.Display(1); bit != 0 && bit <= all; bit <<= 1 {
		if all&bit == 0 {
			continue
		}
		at := 0
		for _, sp := range spans {
			if sp.Display&bit == 0 {
				continue
			}
			if sp.Start > at && len(bytes.TrimSpace(text[at:sp.Start])) > 0 {
				whole &^= bit
				break
			}
			at = max(at, sp.End)
		}
		if whole&bit != 0 && at < len(text) && len(bytes.TrimSpace(text[at:])) > 0 {
			whole &^= bit
		}
	}
	kept := spans[:0]
	for _, sp := range spans {
		sp.Display &^= whole
		if sp.Display != 0 || sp.Style != 0 || sp.Href != "" {
			kept = append(kept, sp)
		}
	}
	if len(kept) == 0 {
		kept = nil
	}
	return string(text), kept, all
}

// cleanText removes what may not be in extracted text and trims it.
func cleanText(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c < 0x20 && c != '\t' && c != '\n' || c >= 0x7F && c < 0xA0 {
			continue
		}
		if c == utf8.RuneError {
			b.WriteRune(c)
			continue
		}
		if l := gofastcore.Ligature(c); l != "" {
			b.WriteString(l)
			continue
		}
		b.WriteRune(c)
	}
	return strings.TrimSpace(b.String())
}

// cleanMarker makes a list marker of the text of \listtext: what is left
// once the tab that ends it and surrounding space are gone.
func cleanMarker(s string) string {
	s = strings.ReplaceAll(cleanText(s), "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// Tables.

type cell struct {
	text   []byte
	hidden gofastcore.Display
	href   string
}

func (c *cell) addPara(text string, hidden gofastcore.Display, spans []gofastcore.Span) {
	if text == "" {
		return
	}
	if len(c.text) > 0 {
		c.text = append(c.text, '\n')
	}
	c.text = append(c.text, text...)
	c.hidden |= hidden
	if c.href == "" {
		for _, s := range spans {
			if s.Href != "" {
				c.href = s.Href
				break
			}
		}
	}
}

type cellDef struct {
	hm, vm uint8 // 1 first of a merge, 2 merged into it
}

type rowDef struct {
	cells  []cellDef
	next   cellDef
	header bool
}

type table struct {
	rows       [][]string
	hidden     []gofastcore.DisplayCells
	details    []gofastcore.CellDetail
	cur        []cell
	c          cell
	merges     []gofastcore.Merge
	vopen      map[int]int
	headerRows int
	pastHeader bool
	after      []gofastcore.Block
}

func (f *flow) ensureTables(level int) {
	for len(f.tables) < level {
		f.tables = append(f.tables, &table{vopen: map[int]int{}})
	}
}

func (f *flow) def(level int) *rowDef {
	for len(f.defs) < level {
		f.defs = append(f.defs, rowDef{})
	}
	return &f.defs[level-1]
}

// endCell ends a cell of the table at the given level (\cell, \nestcell).
func (f *flow) endCell(p para, level int) {
	f.endParaAt(p, level)
	t := f.tables[level-1]
	t.cur = append(t.cur, t.c)
	t.c = cell{}
}

// endRow ends a row of the table at the given level (\row, \nestrow).
func (f *flow) endRow(level int) {
	f.ensureTables(level)
	f.closeDeeper(level)
	t := f.tables[level-1]
	if len(t.c.text) > 0 {
		t.cur = append(t.cur, t.c)
		t.c = cell{}
	}
	if len(t.cur) == 0 {
		return
	}
	if len(t.rows) >= maxRows {
		t.cur = t.cur[:0]
		f.r.cutItems(atBody, "table rows past %d in one table were not read", maxRows)
		return
	}
	cost := 0
	for _, c := range t.cur {
		cost += gofastcore.CellCost + len(c.text) + len(c.href)
	}
	if !f.r.hold(cost) {
		// The rows before this one are kept; reading ends here.
		t.cur = t.cur[:0]
		return
	}
	def := f.def(level)
	ri := len(t.rows)
	row := make([]string, len(t.cur))
	for i, c := range t.cur {
		row[i] = cleanText(string(c.text))
		if c.hidden != 0 && row[i] != "" {
			t.hidden = append(t.hidden, gofastcore.DisplayCells{Row: ri, Col: i, RowSpan: 1, ColSpan: 1, Display: c.hidden})
		}
		if c.href != "" {
			t.details = append(t.details, gofastcore.CellDetail{Row: ri, Col: i, Href: c.href})
		}
		var d cellDef
		if i < len(def.cells) {
			d = def.cells[i]
		}
		switch d.vm {
		case 1:
			t.merges = append(t.merges, gofastcore.Merge{Row: ri, Col: i, RowSpan: 1, ColSpan: 1})
			t.vopen[i] = len(t.merges) - 1
		case 2:
			if m, ok := t.vopen[i]; ok {
				t.merges[m].RowSpan++
			}
		default:
			delete(t.vopen, i)
		}
		if d.hm == 1 {
			span := 1
			for j := i + 1; j < len(t.cur) && j < len(def.cells) && def.cells[j].hm == 2; j++ {
				span++
			}
			if m, ok := t.vopen[i]; ok && d.vm == 1 {
				t.merges[m].ColSpan = span
			} else if span > 1 {
				t.merges = append(t.merges, gofastcore.Merge{Row: ri, Col: i, RowSpan: 1, ColSpan: span})
			}
		}
	}
	if def.header && !t.pastHeader {
		t.headerRows++
	} else {
		t.pastHeader = true
	}
	t.rows = append(t.rows, row)
	t.cur = t.cur[:0]
}

// maxRows bounds the rows of one table; the text budget bounds them too,
// but a row of empty cells costs little text.
const maxRows = 1 << 20

// closeDeeper ends the tables nested deeper than level, writing each into
// the cell of the table it is in, as lines of text with cells separated by
// tabs.
func (f *flow) closeDeeper(level int) {
	for len(f.tables) > level && len(f.tables) > 1 {
		n := len(f.tables)
		t := f.tables[n-1]
		if len(t.c.text) > 0 || len(t.cur) > 0 {
			f.endRow(n)
		}
		f.tables = f.tables[:n-1]
		outer := f.tables[len(f.tables)-1]
		var lines []string
		var hidden gofastcore.Display
		for _, row := range t.rows {
			lines = append(lines, strings.TrimRight(strings.Join(row, "\t"), "\t"))
		}
		for _, h := range t.hidden {
			hidden |= h.Display
		}
		outer.c.addPara(strings.TrimSpace(strings.Join(lines, "\n")), hidden, nil)
		outer.after = append(outer.after, t.after...)
	}
}

// finishTables ends every open table and emits the outermost one.
func (f *flow) finishTables() {
	if len(f.tables) == 0 {
		return
	}
	f.closeDeeper(1)
	t := f.tables[0]
	if len(t.c.text) > 0 || len(t.cur) > 0 {
		f.endRow(1)
	}
	f.tables = f.tables[:0]
	if b, ok := t.block(); ok {
		f.emit(b)
	}
	for _, b := range t.after {
		f.emit(b)
	}
}

// block makes the table's block.
func (t *table) block() (gofastcore.Block, bool) {
	if len(t.rows) == 0 {
		return gofastcore.Block{}, false
	}
	cols := 0
	for _, row := range t.rows {
		cols = max(cols, len(row))
	}
	for i, row := range t.rows {
		for len(row) < cols {
			row = append(row, "")
		}
		t.rows[i] = row
	}
	tb := &gofastcore.Table{Rows: t.rows, HeaderRows: t.headerRows, Merges: sanitizeMerges(t.merges, len(t.rows), cols), Display: t.hidden}
	if len(t.details) > 0 {
		tb.Cells = t.details
	}
	b := gofastcore.Block{Kind: gofastcore.KindTable, Table: tb}
	for _, h := range t.hidden {
		b.Display |= h.Display
	}
	return b, true
}

// sanitizeMerges keeps the merges that lie inside the table and do not
// overlap one before them.
func sanitizeMerges(in []gofastcore.Merge, rows, cols int) []gofastcore.Merge {
	if len(in) == 0 {
		return nil
	}
	used := map[[2]int]bool{}
	var out []gofastcore.Merge
	for _, m := range in {
		m.RowSpan = min(m.RowSpan, rows-m.Row)
		m.ColSpan = min(m.ColSpan, cols-m.Col)
		if m.RowSpan < 1 || m.ColSpan < 1 || m.RowSpan*m.ColSpan < 2 {
			continue
		}
		clash := false
		for r := m.Row; r < m.Row+m.RowSpan && !clash; r++ {
			for c := m.Col; c < m.Col+m.ColSpan; c++ {
				if used[[2]int{r, c}] {
					clash = true
					break
				}
			}
		}
		if clash {
			continue
		}
		for r := m.Row; r < m.Row+m.RowSpan; r++ {
			for c := m.Col; c < m.Col+m.ColSpan; c++ {
				used[[2]int{r, c}] = true
			}
		}
		out = append(out, m)
	}
	return out
}
