// Package reader reads the tokens of an RTF file into the shared document
// model: the group stack with its character and paragraph properties, the
// destinations, and what each one becomes.
package reader

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/AOShei/go-fast-core"
	"github.com/AOShei/go-fast-rtf/internal/codepage"
	"github.com/AOShei/go-fast-rtf/internal/token"
)

// Warning codes of this library.
const (
	// WarnUnreadContent: the file holds content of a kind this version
	// does not read (an equation); the message names it.
	WarnUnreadContent = "rtf.unread_content"
	// WarnControlCharsRemoved: control characters with no meaning as text
	// were removed.
	WarnControlCharsRemoved = "rtf.control_characters_removed"
)

func outlierMessage(n int, what string) string {
	return fmt.Sprintf("%d %s of more than %d words or %d characters, which the document does not show",
		n, what, gofastcore.UnshownOutlierWords, gofastcore.UnshownOutlierChars)
}

// Checked is the set of hidden-text reasons this reader checks completely.
const Checked = gofastcore.HiddenInvisible | gofastcore.HiddenTiny | gofastcore.HiddenLowContrast | gofastcore.HiddenChars

// Config is what the reader is told besides the input.
type Config struct {
	Opts          *gofastcore.Options
	MaxGroupDepth int // groups nested deeper are skipped
	Decode        *gofastcore.DecodeBudget
	Text          *gofastcore.TextBudget
}

// chr holds the character properties.
type chr struct {
	font              int32
	fs                int32 // half-points
	cf, cb, hl, shd   int32 // colour table indexes; 0 is auto or none
	hidden, webHidden bool
	deleted           bool
	bold, italic, ul  bool
	strike, sup, sub  bool
	revDel, dttmDel   int32
}

// para holds the paragraph properties.
type para struct {
	style   int32
	outline int32 // -1 none
	ls      int32
	ilvl    int32
	itap    int32
	intbl   bool
	shd     int32 // \cbpat: colour index of the paragraph's shading
	bdr     uint8 // sides with a border that has a style
	bpend   uint8 // sides the border words being read are for
}

// Sides of a paragraph's border.
const (
	borderTop uint8 = 1 << iota
	borderLeft
	borderRight
	borderBottom
	borderBetween
	borderBar
)

// dest says what kind of destination a group is, where that matters to
// how its control words and text are read.
type dest uint8

const (
	dBody dest = iota
	dFontTbl
	dColorTbl
	dStyleSheet
	dListTable
	dListOverrideTable
	dInfo
	dDate
	dRevTbl
	dPict
	dObjData
	dWords // control words are read, text is not
	dUserProps
	dDocVar
)

// group is the state a group saves and restores.
type group struct {
	c       chr
	p       para
	uc      int32
	dest    dest
	flow    *flow   // paragraph-structured sink, or nil
	buf     *strbuf // plain-text sink, or nil
	href    string  // link of the field result being read
	star    bool    // the group began with \*
	first   bool    // no control word has been read in the group yet
	onClose func()
	uprSkip bool      // the next child group is the ANSI half of \upr
	nest    bool      // inside \nesttableprops
	style   *style    // the style being defined, in the style sheet
	kids    []*strbuf // where the text of the next child groups goes
	kidN    int
}

// strbuf collects plain text up to a bound.
type strbuf struct {
	b     []byte
	limit int
	full  bool
}

func newBuf(limit int) *strbuf { return &strbuf{limit: limit} }

func (s *strbuf) addRune(r rune) {
	if len(s.b)+4 > s.limit {
		s.full = true
		return
	}
	s.b = appendRune(s.b, r)
}

func (s *strbuf) String() string { return string(s.b) }

type font struct {
	cp  int
	cpg int // set by \cpg, which wins over \fcharset
}

type listDef struct {
	id     int32
	hasID  bool
	levels []int32
	open   bool
}

type rgb struct {
	r, g, b uint8
	auto    bool
}

type style struct {
	num     int32
	name    *strbuf
	outline int32
	basedOn int32
	char    bool // a character, section or table style, not a paragraph style
	cs      bool // a character style
	// The character properties the style sets: those in set.
	set       uint8
	hidden    bool
	webHidden bool
	c         chr
}

type reader struct {
	ctx   context.Context
	cfg   Config
	opts  *gofastcore.Options
	lex   *token.Lexer
	doc   *gofastcore.Document
	part  *gofastcore.Part
	store *gofastcore.Part

	stack []group
	skip  int // groups being skipped, when above zero
	// firstWord: the control word being read is the first of its group
	firstWord bool

	body *flow

	ansicpg  int
	deff     int32
	fonts    map[int32]font
	curFont  int32 // font being defined in the font table
	colors   []rgb
	color    rgb
	colorAny bool
	styles   map[int32]*style  // paragraph styles (and section and table styles)
	cstyles  map[int32]*style  // character styles
	lists    map[int32][]int32 // list id -> \levelnfc of each level
	lsList   map[int32]int32   // \ls -> list id
	revs     []string
	page     *rgb // the page's background colour, if the file sets one
	lang     int32

	form        *formField
	curList     *listDef
	curOverride *[3]int32 // list id, \ls, list id set
	bgDepth     int
	props       [][2]*strbuf
	docvars     [][2]string // text, name
	userprops   [][2]string
	unsupported int
	// unshown items out of proportion for their kind, in the body and in
	// the stored part
	outliers, storeOutliers int

	dec      codepage.Decoder
	ucLeft   int32
	hiSurr   rune
	unmapped map[int]int
	removed  int
	contrast map[[2]int32]bool

	fields           []*field
	shape            *shape
	pict             *pict
	sp               *spState
	obj              *object
	date             *dateBuf
	atnAuthor, atnID string
	ftnNum, endNum   int
	ftnFmt, endFmt   string

	// counts for warnings
	deleted, inserted int
	clamped           int
	external, active  int
	unread            map[string]int
	sawImage          bool
	pictN             int
	textCut, decCut   bool
	depthCut          bool
	repaired          string
	delivered         map[string]bool

	// metadata being read
	info      map[string]*strbuf
	dates     map[string]*dateBuf
	generator *strbuf
	trailing  *strbuf
	ended     bool
}

// Read reads an RTF stream into a document. The stream must begin with
// "{\rtf"; the caller has checked that.
func Read(ctx context.Context, src io.Reader, cfg Config) (*gofastcore.Document, error) {
	r := &reader{
		ctx: ctx, cfg: cfg, opts: cfg.Opts,
		lex:       token.New(src),
		doc:       gofastcore.New("rtf"),
		part:      &gofastcore.Part{Kind: gofastcore.PartBody, Number: 1},
		store:     &gofastcore.Part{Kind: gofastcore.PartStored, Number: 2},
		ansicpg:   1252,
		fonts:     map[int32]font{},
		styles:    map[int32]*style{},
		cstyles:   map[int32]*style{},
		lists:     map[int32][]int32{},
		lsList:    map[int32]int32{},
		unmapped:  map[int]int{},
		contrast:  map[[2]int32]bool{},
		unread:    map[string]int{},
		delivered: map[string]bool{},
		info:      map[string]*strbuf{},
		dates:     map[string]*dateBuf{},
		ftnFmt:    "arabic", endFmt: "roman",
	}
	if r.opts == nil {
		r.opts = &gofastcore.Options{}
	}
	r.body = &flow{r: r, kind: flowBody}
	if err := r.run(); err != nil {
		return nil, err
	}
	r.finish()
	return r.doc, nil
}

func defaultChr() chr { return chr{fs: 24} }

func defaultPara() para { return para{outline: -1} }

func (r *reader) top() *group { return &r.stack[len(r.stack)-1] }

func (r *reader) run() error {
	n := 0
	for !r.textCut {
		if n++; n&4095 == 0 {
			if err := r.ctx.Err(); err != nil {
				return err
			}
		}
		r.lex.KeepBin = r.keepBin()
		t := r.lex.Next()
		if t.Kind == token.EOF {
			break
		}
		if r.skip > 0 {
			switch t.Kind {
			case token.Open:
				r.skip++
			case token.Close:
				r.skip--
			}
			continue
		}
		if r.ended {
			r.afterEnd(t)
			continue
		}
		switch t.Kind {
		case token.Open:
			r.open()
		case token.Close:
			r.close()
		case token.Word:
			r.word(t)
		case token.Symbol:
			r.symbol(t.Sym)
		case token.Text:
			r.text(t.Data)
		case token.Hex:
			r.hex(t.Sym)
		case token.Bin:
			r.bin(t)
		}
	}
	if err := r.lex.Err(); err != nil && r.lex.Read == 0 {
		return err
	}
	if !r.ended && !r.textCut {
		depth := len(r.stack) + r.skip
		r.repaired = fmt.Sprintf("the file ends inside %d groups; it was read as far as it goes", depth)
		r.skip = 0
		for len(r.stack) > 0 {
			r.close()
		}
	}
	return nil
}

// keepBin is how many bytes of the data of a \binN to keep.
func (r *reader) keepBin() int64 {
	if r.skip > 0 || len(r.stack) == 0 {
		return 0
	}
	switch r.top().dest {
	case dPict:
		if p := r.pict; p != nil {
			return p.room()
		}
	case dObjData:
		if o := r.obj; o != nil {
			return o.room()
		}
	}
	return 0
}

// open begins a group.
func (r *reader) open() {
	r.flushBytes()
	r.ucLeft = 0
	if len(r.stack) == 0 {
		g := group{c: defaultChr(), p: defaultPara(), uc: 1, flow: r.body, first: true}
		r.stack = append(r.stack, g)
		return
	}
	parent := r.top()
	if parent.uprSkip {
		parent.uprSkip = false
		r.skip = 1
		return
	}
	if len(r.stack) >= r.cfg.MaxGroupDepth {
		r.depthCut = true
		r.skip = 1
		return
	}
	g := *parent
	g.onClose, g.star, g.first, g.uprSkip, g.kids, g.kidN = nil, false, true, false, nil, 0
	switch {
	case parent.kids != nil:
		if parent.kidN < len(parent.kids) {
			g.buf, g.dest = parent.kids[parent.kidN], dBody
			parent.kidN++
		} else {
			g.buf = newBuf(0)
		}
	case parent.dest == dStyleSheet && parent.style == nil:
		st := &style{outline: -1, basedOn: -1, name: newBuf(512)}
		g.style, g.buf = st, st.name
		g.onClose = func() {
			if st.cs {
				if len(r.cstyles) < 1<<16 {
					r.cstyles[st.num] = st
				}
			} else if len(r.styles) < 1<<16 {
				if _, dup := r.styles[st.num]; !dup || !st.char {
					r.styles[st.num] = st
				}
			}
		}
	case parent.dest == dRevTbl && parent.buf == nil:
		b := newBuf(512)
		g.buf = b
		g.onClose = func() {
			if len(r.revs) < 1<<14 {
				r.revs = append(r.revs, strings.TrimSuffix(cleanText(b.String()), ";"))
			}
		}
	}
	r.stack = append(r.stack, g)
}

// close ends a group.
func (r *reader) close() {
	r.flushBytes()
	r.ucLeft = 0
	if len(r.stack) == 0 {
		return
	}
	g := r.top()
	if f := g.onClose; f != nil {
		g.onClose = nil
		f()
	}
	r.stack = r.stack[:len(r.stack)-1]
	if len(r.stack) == 0 {
		r.body.end()
		r.ended = true
	}
}

// skipGroup drops the group just begun, and everything in it. A
// destination word that does not begin its group, or that stands in the
// document's own group, is ignored instead.
func (r *reader) skipGroup() {
	if !r.firstWord || len(r.stack) <= 1 {
		return
	}
	r.stack = r.stack[:len(r.stack)-1]
	r.skip = 1
}

// afterEnd reads what follows the closing brace of the document. No
// program shows it; its text is kept in the stored part.
func (r *reader) afterEnd(t token.Token) {
	if t.Kind != token.Text && t.Kind != token.Hex {
		return
	}
	if r.trailing == nil {
		r.trailing = newBuf(1 << 20)
	}
	emit := func(c rune, ok bool) {
		if !ok {
			r.unmapped[r.ansicpg]++
		}
		r.trailing.addRune(c)
	}
	if t.Kind == token.Hex {
		r.dec.Byte(r.ansicpg, t.Sym, emit)
		return
	}
	for _, b := range t.Data {
		r.dec.Byte(r.ansicpg, b, emit)
	}
}

// codePage is the code page of text in the current font.
func (r *reader) codePage() int {
	g := r.top()
	if f, ok := r.fonts[g.c.font]; ok && f.cp != 0 {
		return f.cp
	}
	return r.ansicpg
}

// text reads a run of text bytes.
func (r *reader) text(data []byte) {
	g := r.top()
	switch g.dest {
	case dPict:
		if r.pict != nil {
			r.pict.hex(data)
		}
		return
	case dObjData:
		if r.obj != nil {
			r.obj.hex(data)
		}
		return
	case dColorTbl:
		for _, b := range data {
			if b == ';' {
				r.endColor()
			}
		}
		return
	case dFontTbl, dListTable, dListOverrideTable, dWords, dRevTbl:
		if g.dest == dRevTbl && g.buf != nil {
			break
		}
		return
	case dStyleSheet:
		if g.buf == nil {
			return
		}
	}
	if r.ucLeft > 0 {
		n := min(int(r.ucLeft), len(data))
		r.ucLeft -= int32(n)
		data = data[n:]
	}
	if len(data) == 0 {
		return
	}
	cp := r.codePage()
	for _, b := range data {
		if b == '\t' {
			r.flushBytes()
			r.put('\t')
			continue
		}
		if b < 0x20 {
			r.removed++
			continue
		}
		r.dec.Byte(cp, b, r.emitDecoded(cp))
	}
}

func (r *reader) emitDecoded(cp int) func(rune, bool) {
	return func(c rune, ok bool) {
		if !ok {
			r.unmapped[cp]++
		}
		r.put(c)
	}
}

// hex reads \'hh.
func (r *reader) hex(b byte) {
	g := r.top()
	switch g.dest {
	case dPict, dObjData, dColorTbl, dFontTbl, dListTable, dListOverrideTable, dWords:
		return
	}
	if r.ucLeft > 0 {
		r.ucLeft--
		return
	}
	cp := r.codePage()
	r.dec.Byte(cp, b, r.emitDecoded(cp))
}

// flushBytes ends a run of bytes: a lead byte left alone is not decoded.
func (r *reader) flushBytes() {
	if len(r.stack) == 0 {
		return
	}
	r.dec.Flush(func(c rune, ok bool) {
		r.unmapped[r.codePage()]++
		r.put(c)
	})
}

// bin reads the data of \binN.
func (r *reader) bin(t token.Token) {
	if r.ucLeft > 0 {
		r.ucLeft--
		return
	}
	switch r.top().dest {
	case dPict:
		if r.pict != nil {
			r.pict.raw(t.Data, t.BinLen)
		}
	case dObjData:
		if r.obj != nil {
			r.obj.raw(t.Data, t.BinLen)
		}
	}
}

// put sends one character to where the current group's text goes.
func (r *reader) put(c rune) {
	if r.hiSurr != 0 {
		r.hiSurr = 0
		r.put1(codepage.Unmapped)
	}
	r.put1(c)
}

func (r *reader) put1(c rune) {
	if c < 0x20 && c != '\t' && c != '\n' || c >= 0x7F && c < 0xA0 {
		r.removed++
		return
	}
	if c == 0xAD { // soft hyphen: shown only where a line breaks
		return
	}
	g := r.top()
	if g.buf != nil {
		g.buf.addRune(c)
		return
	}
	if g.flow == nil || g.dest != dBody {
		return
	}
	g.flow.add(c, g)
}

// unicode reads \uN.
func (r *reader) unicode(v int32) {
	c := rune(v)
	if c < 0 {
		c += 65536
	}
	r.flushBytes()
	r.ucLeft = r.top().uc
	switch {
	case c >= 0xD800 && c < 0xDC00:
		if r.hiSurr != 0 {
			r.put1(codepage.Unmapped)
		}
		r.hiSurr = c
		return
	case c >= 0xDC00 && c < 0xE000:
		if r.hiSurr != 0 {
			hi := r.hiSurr
			r.hiSurr = 0
			r.put1((hi-0xD800)<<10 | (c - 0xDC00) + 0x10000)
			return
		}
		r.put1(codepage.Unmapped)
		r.unmapped[-1]++
		return
	case c > 0x10FFFF:
		c = codepage.Unmapped
	}
	r.put(c)
}

// symbol reads a control symbol.
func (r *reader) symbol(s byte) {
	if s == '*' {
		r.top().star = true
		return
	}
	r.flushBytes()
	if r.ucLeft > 0 {
		r.ucLeft--
		return
	}
	switch s {
	case '\\', '{', '}':
		g := r.top()
		if g.dest == dBody || g.buf != nil {
			r.put(rune(s))
		}
	case '~':
		r.put(0xA0)
	case '_':
		r.put(0x2011)
	case '-', ':', '|':
	}
	r.top().first = false
}

// finish puts together the document once the input is read.
func (r *reader) finish() {
	r.body.end()
	part := r.part
	r.metadata()
	hasText, hasImage := false, false
	for i := range part.Blocks {
		switch b := &part.Blocks[i]; {
		case b.Kind == gofastcore.KindImage:
			hasImage = true
		case b.Text != "" || b.Table != nil:
			hasText = true
		}
	}
	if r.repaired != "" {
		r.doc.Warn(gofastcore.SeverityInfo, gofastcore.WarnInputRepaired, r.repaired)
	}
	if r.textCut {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the text limit was reached; the document is cut off there")
	}
	if r.decCut {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the decoded data limit was reached; pictures or embedded objects past it have no data")
	}
	if r.depthCut {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, fmt.Sprintf("groups nested deeper than %d levels were not read", r.cfg.MaxGroupDepth))
	}
	if !hasText {
		switch {
		case hasImage || r.sawImage:
			part.Warn(gofastcore.SeverityWarning, gofastcore.WarnPartImageOnly, "")
		default:
			part.Warn(gofastcore.SeverityInfo, gofastcore.WarnPartNoText, "")
		}
	}
	if r.inserted+r.deleted > 0 {
		part.Warn(gofastcore.SeverityInfo, gofastcore.WarnTextTrackedChanges,
			strconv.Itoa(r.inserted)+" insertions, "+strconv.Itoa(r.deleted)+" deletions; text is given with all changes accepted")
	}
	if r.outliers > 0 {
		part.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextUnshownOutlier, outlierMessage(r.outliers, "field instructions or index entries"))
	}
	if r.removed > 0 {
		part.Warn(gofastcore.SeverityInfo, WarnControlCharsRemoved, strconv.Itoa(r.removed))
	}
	r.unmappedWarnings(part)
	if r.unsupported > 0 {
		part.Warn(gofastcore.SeverityWarning, gofastcore.WarnImageUnsupportedEncoding,
			strconv.Itoa(r.unsupported)+" pictures in a device-dependent bitmap, OS/2 metafile or unnamed format have no data")
	}
	if r.clamped > 0 {
		part.Warn(gofastcore.SeverityInfo, gofastcore.WarnHeadingLevelClamped, strconv.Itoa(r.clamped)+" headings deeper than level 6")
	}
	if len(r.unread) > 0 {
		names := make([]string, 0, len(r.unread))
		for k, n := range r.unread {
			names = append(names, fmt.Sprintf("%s (%d)", k, n))
		}
		sort.Strings(names)
		part.Warn(gofastcore.SeverityWarning, WarnUnreadContent, strings.Join(names, ", "))
	}
	if r.external > 0 {
		r.doc.Warn(gofastcore.SeverityWarning, gofastcore.WarnInputExternalReference, fmt.Sprintf("%d, not counting hyperlinks; nothing was fetched", r.external))
	}
	if r.active > 0 {
		r.doc.Warn(gofastcore.SeverityWarning, gofastcore.WarnInputActiveContent, fmt.Sprintf("%d fields or controls that run commands or programs; nothing was run", r.active))
	}
	part.MarkHidden()
	r.doc.Parts = append(r.doc.Parts, *part)
	r.storedPart()
	r.doc.HiddenChecked = Checked
}

func (r *reader) unmappedWarnings(part *gofastcore.Part) {
	cps := make([]int, 0, len(r.unmapped))
	for cp := range r.unmapped {
		cps = append(cps, cp)
	}
	sort.Ints(cps)
	for _, cp := range cps {
		n := r.unmapped[cp]
		var msg string
		switch {
		case cp == -1:
			msg = fmt.Sprintf("%d unpaired surrogate code units in \\u escapes are U+FFFD", n)
		case cp == codepage.Symbol:
			msg = fmt.Sprintf("%d characters in Symbol fonts are U+FFFD", n)
		case codepage.DoubleByte(cp):
			msg = fmt.Sprintf("%d characters have no mapping in code page %d, or are a lead byte with no trail byte, and are U+FFFD", n, cp)
		case !codepage.Known(cp):
			msg = fmt.Sprintf("code page %d was not decoded: %d bytes are U+FFFD", cp, n)
		default:
			msg = fmt.Sprintf("%d bytes have no character in code page %d and are U+FFFD", n, cp)
		}
		part.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextUnmappedGlyphs, msg)
	}
}

// add appends a finished block to the body part, taking its cost from the
// text budget.
func (r *reader) add(b gofastcore.Block) {
	if r.textCut {
		return
	}
	cost := gofastcore.BlockCost + len(b.Text) + len(b.Marker)
	for _, s := range b.Spans {
		cost += len(s.Href)
	}
	if t := b.Table; t != nil {
		for _, row := range t.Rows {
			for _, c := range row {
				cost += gofastcore.CellCost + len(c)
			}
		}
		for _, c := range t.Cells {
			cost += len(c.Href)
		}
	}
	if !r.cfg.Text.Take(cost) {
		r.textCut = true
		return
	}
	r.part.Blocks = append(r.part.Blocks, b)
}

// storedTrailing is Attrs["stored"] of the text after the document's
// closing brace.
const storedTrailing = "trailing_data"

// addStored appends a block to the stored part.
func (r *reader) addStored(text, kind, name string) {
	text = cleanText(text)
	if text == "" || r.textCut {
		return
	}
	attrs := map[string]string{gofastcore.AttrStored: kind}
	if name != "" {
		attrs[gofastcore.AttrName] = name
	}
	b := gofastcore.Block{Kind: gofastcore.KindParagraph, Text: text, Attrs: attrs}
	switch {
	case kind == storedTrailing:
		// Clean files have nothing after the closing brace: it is a
		// signal at any length, not an outlier of routine data.
		b.Hidden = gofastcore.HiddenInvisible
	case gofastcore.UnshownOutlier(text):
		b.Hidden = gofastcore.HiddenInvisible
		r.storeOutliers++
	}
	if !r.cfg.Text.Take(gofastcore.BlockCost + len(text) + len(name)) {
		r.textCut = true
		r.store.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the text limit was reached; the stored data is cut off there")
		return
	}
	r.store.Blocks = append(r.store.Blocks, b)
}

func (r *reader) storedPart() {
	if r.trailing != nil {
		r.addStored(r.trailing.String(), storedTrailing, "")
	}
	if len(r.store.Blocks) == 0 {
		return
	}
	if !r.opts.Parts.Contains(2) || r.opts.Parts != (gofastcore.PartRange{}) {
		return
	}
	if m := r.opts.Limits.MaxParts; m > 0 && m < 2 {
		r.doc.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the part limit was reached; the stored part is left out")
		return
	}
	if r.storeOutliers > 0 {
		r.store.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextUnshownOutlier, outlierMessage(r.storeOutliers, "stored items"))
	}
	r.store.MarkHidden()
	r.doc.Parts = append(r.doc.Parts, *r.store)
}
