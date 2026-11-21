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
	"unicode/utf8"

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

// Config is what the reader is told besides the input.
type Config struct {
	Opts          *gofastcore.Options
	MaxGroupDepth int // groups nested deeper are skipped
	Decode        *gofastcore.DecodeBudget
	Text          *gofastcore.TextBudget
	// DropBody: the body gives no blocks, because the document's content
	// was read another way (the HTML it encapsulates).
	DropBody bool
}

// chr holds the character properties.
type chr struct {
	font             int32
	hidden           bool // \v
	deleted          bool
	bold, italic, ul bool
	strike, sup, sub bool
	revDel, dttmDel  int32
}

// para holds the paragraph properties.
type para struct {
	style   int32
	outline int32 // -1 none
	ls      int32
	ilvl    int32
	itap    int32
	intbl   bool
}

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

// strbuf collects plain text up to a bound. A buffer of limit 0 takes
// nothing: it is where text that is not kept goes.
type strbuf struct {
	b     []byte
	limit int
	full  bool  // text was left out at the bound
	lost  int64 // the bytes of UTF-8 left out
}

func newBuf(limit int) *strbuf { return &strbuf{limit: limit} }

// addRune appends r if it fits in the bound. Once a rune does not fit,
// none after it is taken either: the text is cut there.
func (s *strbuf) addRune(r rune) {
	n := runeBytes(r)
	if s.full || len(s.b)+n > s.limit {
		s.full = true
		s.lost += int64(n)
		return
	}
	s.b = appendRune(s.b, r)
}

// runeBytes is the number of bytes appendRune writes for c.
func runeBytes(c rune) int {
	if l := gofastcore.Ligature(c); l != "" {
		return len(l)
	}
	if n := utf8.RuneLen(c); n > 0 {
		return n
	}
	return utf8.RuneLen(utf8.RuneError) // what an invalid rune is written as
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

type style struct {
	num     int32
	name    *strbuf
	outline int32
	basedOn int32
	char    bool // a character, section or table style, not a paragraph style
	cs      bool // a character style
	// hidden is the style's \v, if it sets it (setsHidden).
	setsHidden bool
	hidden     bool
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

	ansicpg int
	deff    int32
	fonts   map[int32]font
	// fontDropped: the font table entry being read names a font past
	// maxDefs; it is counted when the entry's group ends.
	fontDropped bool
	curFont     int32             // font being defined in the font table
	styles      map[int32]*style  // paragraph styles (and section and table styles)
	cstyles     map[int32]*style  // character styles
	lists       map[int32][]int32 // list id -> \levelnfc of each level
	lsList      map[int32]int32   // \ls -> list id
	revs        []string
	lang        int32

	form        *formField
	curList     *listDef
	curOverride *[3]int32 // list id, \ls, list id set
	props       [][2]*strbuf
	propDropped bool        // the custom property being read is past maxStoredItems
	docvars     [][2]string // text, name
	userprops   [][2]string
	unsupported int
	// endAt is the offset just after the document's closing brace;
	// trailingData says that more than white space follows it.
	endAt        int64
	trailingData bool

	dec      codepage.Decoder
	ucLeft   int32
	hiSurr   rune
	unmapped map[int]int // replacements by code page; see miss
	// unmappedMore counts the replacements in code pages met once
	// unmapped held maxCodePages of them.
	unmappedMore int
	removed      int

	fields           []*field
	shape            *shape
	pict             *pict
	sp               *spState
	obj              *object
	date             *dateBuf
	atnAuthor, atnID string
	// the buffers atnAuthor and atnID were read from, to report a cut
	atnAuthorCut, atnIDCut *strbuf
	ftnNum, endNum         int
	ftnFmt, endFmt         string

	// counts for warnings
	deleted, inserted int
	clamped           int
	external, active  int
	unread            map[string]int
	sawImage          bool
	pictN             int
	textCut, decCut   bool
	storeCut          bool // the text limit was reached in the stored data; no more is added
	// held is the cost of the blocks and table rows read but not yet
	// added, since the last paragraph of the body ended; stop is set when
	// it would not fit the text budget, and reading ends there.
	held      int
	stop      bool
	depthCut  bool
	repaired  string
	delivered map[string]bool
	// cuts are the fixed bounds of this library that left content out,
	// each reported once, on the part that lost it.
	cuts []capCut

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

func defaultChr() chr { return chr{} }

func defaultPara() para { return para{outline: -1} }

func (r *reader) top() *group { return &r.stack[len(r.stack)-1] }

func (r *reader) run() error {
	n := 0
	for !r.textCut && !r.stop {
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
	if !r.ended && !r.textCut && !r.stop {
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
			table := r.styles
			if st.cs {
				table = r.cstyles
			}
			_, dup := table[st.num]
			switch {
			case !dup && len(table) >= maxDefs:
				r.cutItems(atBody, "styles past %d in the style sheet were not read; text in them takes no heading level or hidden property from them", maxDefs)
			case st.cs || !dup || !st.char:
				table[st.num] = st
			}
		}
	case parent.dest == dRevTbl && parent.buf == nil:
		b := newBuf(512)
		g.buf = b
		g.onClose = func() {
			if len(r.revs) < maxRevs {
				r.revs = append(r.revs, strings.TrimSuffix(cleanText(b.String()), ";"))
				r.cutBuf(atBody, "revision table entries (author names) are cut at", b)
			} else {
				r.cutItems(atBody, "revision table entries (author names) past %d were not read", maxRevs)
			}
		}
	}
	r.stack = append(r.stack, g)
}

// maxDefs bounds the fonts, the paragraph styles, the character styles,
// the lists and the list overrides that are kept, each.
const maxDefs = 1 << 16

// maxCodePages bounds the code pages text.unmapped_glyphs counts one by
// one; the rest are counted together.
const maxCodePages = 64

// maxRevs bounds the entries of the revision table that are kept.
const maxRevs = 1 << 14

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
	if g.dest == dFontTbl && r.fontDropped {
		r.fontDropped = false
		r.cutItems(atBody, "fonts past %d in the font table were not read; text in them is decoded in the document's code page", maxDefs)
	}
	r.stack = r.stack[:len(r.stack)-1]
	if len(r.stack) == 0 {
		r.body.end()
		r.ended, r.endAt = true, r.lex.Read
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

// afterEnd reads what follows the closing brace of the document. Its text
// is kept in the stored part, as after_end, with the warning
// input.trailing_data. Only white space, NUL bytes and 0x1A (Ctrl-Z, the
// DOS end-of-file mark) are ignored: when nothing else follows the brace
// there is neither a stored block nor a warning. There is no threshold on
// how much else may follow; any other byte counts.
func (r *reader) afterEnd(t token.Token) {
	switch t.Kind {
	case token.Text:
		for _, b := range t.Data {
			if b != ' ' && b != '\t' && b != '\r' && b != '\n' && b != 0 && b != 0x1A {
				r.trailingData = true
				break
			}
		}
	case token.Hex:
		r.trailingData = true
	default:
		r.trailingData = true
		return
	}
	if r.trailing == nil {
		r.trailing = newBuf(1 << 20)
	}
	emit := func(c rune, ok bool) {
		if !ok {
			r.miss(r.ansicpg)
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
	case dColorTbl, dFontTbl, dListTable, dListOverrideTable, dWords, dRevTbl:
		if g.dest == dRevTbl && g.buf != nil {
			break
		}
		return
	case dStyleSheet:
		if g.buf == nil {
			return
		}
	}
	if r.dropped(g) {
		return
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
			r.miss(cp)
		}
		r.put(c)
	}
}

// dropped says that text read in g is not kept: the body gives no
// blocks, and g is body text.
func (r *reader) dropped(g *group) bool {
	return r.cfg.DropBody && g.buf == nil && g.dest == dBody
}

// hex reads \'hh.
func (r *reader) hex(b byte) {
	g := r.top()
	switch g.dest {
	case dPict, dObjData, dColorTbl, dFontTbl, dListTable, dListOverrideTable, dWords:
		return
	}
	if r.dropped(g) {
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
		r.miss(r.codePage())
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
	if g.flow == nil || g.dest != dBody || r.cfg.DropBody {
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
		r.miss(-1)
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
	if r.textCut || r.stop {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the text limit was reached; the document is cut off there")
	}
	if r.decCut {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the decoded data limit was reached; pictures or embedded objects past it have no data")
	}
	if r.depthCut {
		part.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, fmt.Sprintf("groups nested deeper than %d levels were not read", r.cfg.MaxGroupDepth))
	}
	if !r.cfg.DropBody { // else the body's blocks are the HTML's
		r.cutWarnings(atBody, part.Warn)
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
	part.Finish()
	r.doc.Parts = append(r.doc.Parts, *part)
	r.storedPart()
	r.cutWarnings(atDoc, r.doc.Warn)
}

// maxListed is how many code pages a text.unmapped_glyphs warning names
// one by one.
const maxListed = 8

// miss counts a U+FFFD given for text in code page cp: -1 for an unpaired
// surrogate, codepage.Symbol for a Symbol font. The first maxCodePages
// code pages met are counted one by one, and the two negative values
// always; replacements in the code pages met after them are counted
// together, so that a file switching among many code pages takes no more
// memory.
func (r *reader) miss(cp int) {
	if _, ok := r.unmapped[cp]; ok || cp < 0 || len(r.unmapped) < maxCodePages {
		r.unmapped[cp]++
		return
	}
	r.unmappedMore++
}

// unmappedWarnings gives one text.unmapped_glyphs warning for the part,
// whatever the number of code pages: with one, it says what happened in
// it; with more, it gives the total and says it for each of the first
// maxListed, in the order of their numbers, and counts the rest. With
// more than maxCodePages, the first maxListed are those of the code pages
// counted one by one (see miss), and the rest are at least so many.
func (r *reader) unmappedWarnings(part *gofastcore.Part) {
	if len(r.unmapped) == 0 {
		return
	}
	cps := make([]int, 0, len(r.unmapped))
	for cp := range r.unmapped {
		cps = append(cps, cp)
	}
	sort.Ints(cps)
	if len(cps) == 1 && r.unmappedMore == 0 {
		part.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextUnmappedGlyphs, unmappedMessage(cps[0], r.unmapped[cps[0]]))
		return
	}
	total, listed := r.unmappedMore, 0
	msgs := make([]string, 0, maxListed)
	for i, cp := range cps {
		n := r.unmapped[cp]
		total += n
		if i < maxListed {
			listed += n
			msgs = append(msgs, unmappedMessage(cp, n))
		}
	}
	msg := fmt.Sprintf("%d characters or bytes in all are U+FFFD: %s", total, strings.Join(msgs, "; "))
	switch more := len(cps) - maxListed; {
	case r.unmappedMore > 0:
		// The code pages past maxCodePages are not told apart: at least
		// one of them was met.
		msg += fmt.Sprintf("; and at least %d more code pages (past the first %d met they are not told apart), with %d more U+FFFD", max(more, 0)+1, maxCodePages, total-listed)
	case more > 0:
		msg += fmt.Sprintf("; and %d more code pages, with %d more U+FFFD", more, total-listed)
	}
	part.Warn(gofastcore.SeverityWarning, gofastcore.WarnTextUnmappedGlyphs, msg)
}

func unmappedMessage(cp, n int) string {
	switch {
	case cp == -1:
		return fmt.Sprintf("%d unpaired surrogate code units in \\u escapes are U+FFFD", n)
	case cp == codepage.Symbol:
		return fmt.Sprintf("%d characters in Symbol fonts are U+FFFD", n)
	case codepage.DoubleByte(cp):
		return fmt.Sprintf("%d characters have no mapping in code page %d, or are a lead byte with no trail byte, and are U+FFFD", n, cp)
	case !codepage.Known(cp):
		return fmt.Sprintf("code page %d was not decoded: %d bytes are U+FFFD", cp, n)
	}
	return fmt.Sprintf("%d bytes have no character in code page %d and are U+FFFD", n, cp)
}

// Where a fixed bound of this library left content out.
const (
	atDoc   uint8 = iota // the document: metadata, embedded objects' names
	atBody               // the body part
	atStore              // the stored part, or the document without one
)

// capCut counts what one fixed bound left out.
type capCut struct {
	at    uint8
	what  string // with limit, says what the bound is; see cutWarnings
	limit int
	n     int64
	bytes int64 // the bytes of text left out; -1 when n counts items
}

// cut records that the bound limit cut n strings, leaving out that many
// bytes of text, or, with bytes negative, that it left out n items.
func (r *reader) cut(at uint8, what string, limit int, n, bytes int64) {
	for i := range r.cuts {
		if c := &r.cuts[i]; c.at == at && c.what == what && c.limit == limit {
			c.n += n
			if bytes >= 0 {
				c.bytes += bytes
			}
			return
		}
	}
	r.cuts = append(r.cuts, capCut{at: at, what: what, limit: limit, n: n, bytes: bytes})
}

// cutItems records that the bound limit left out an item. what holds a %d
// for the bound: "table rows past %d in one table were not read".
func (r *reader) cutItems(at uint8, what string, limit int) { r.cut(at, what, limit, 1, -1) }

// cutBuf records that b was cut at its bound, if it was. what says what
// the buffer holds and is followed by the bound: "field instructions are
// cut at".
func (r *reader) cutBuf(at uint8, what string, b *strbuf) {
	if b != nil && b.full && b.limit > 0 {
		r.cut(at, what, b.limit, 1, b.lost)
	}
}

// cutWarnings gives one limit.exceeded warning for each bound that left
// out content reported at at.
func (r *reader) cutWarnings(at uint8, warn func(gofastcore.Severity, string, string)) {
	for _, c := range r.cuts {
		if c.at != at {
			continue
		}
		var msg string
		if c.bytes >= 0 {
			msg = fmt.Sprintf("%s %d bytes: %d cut, %d bytes left out", c.what, c.limit, c.n, c.bytes)
		} else {
			msg = fmt.Sprintf(c.what, c.limit) + fmt.Sprintf(" (%d)", c.n)
		}
		warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, msg)
	}
}

// hold counts the cost of a block or table row that is kept until its
// paragraph or table ends, and reports whether it fits what is left of the
// text budget. When it does not, reading stops: what is held is added as
// far as the budget goes.
func (r *reader) hold(cost int) bool {
	if int64(r.held+cost+gofastcore.BlockCost) > r.cfg.Text.Left() {
		r.stop = true
		return false
	}
	r.held += cost
	return true
}

// holdBlock is hold for a block.
func (r *reader) holdBlock(b *gofastcore.Block) bool {
	return r.hold(gofastcore.BlockCost + len(b.Text) + len(b.Marker))
}

// add appends a finished block to the body part, taking its cost from the
// text budget.
func (r *reader) add(b gofastcore.Block) {
	if r.textCut || r.cfg.DropBody {
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

// addStored appends a block to the stored part. Stored data carries no
// display reason: it is where it came from, and the file says no more.
func (r *reader) addStored(text, kind, name string) {
	text = cleanText(text)
	if text == "" || r.textCut || r.storeCut {
		return
	}
	attrs := map[string]string{gofastcore.AttrStored: kind}
	if name != "" {
		attrs[gofastcore.AttrName] = name
	}
	b := gofastcore.Block{Kind: gofastcore.KindParagraph, Text: text, Attrs: attrs}
	if !r.cfg.Text.Take(gofastcore.BlockCost + len(text) + len(name)) {
		// The body is complete: this stops only the stored data.
		r.storeCut = true
		return
	}
	r.store.Blocks = append(r.store.Blocks, b)
}

func (r *reader) storedPart() {
	if r.trailing != nil && r.trailingData {
		r.addStored(r.trailing.String(), gofastcore.StoredAfterEnd, "")
		r.cutBuf(atStore, "the text after the end of the document is cut at", r.trailing)
	}
	// Data after the closing brace is stated in a warning on the stored
	// part, or on the document when there is no stored part to carry it.
	trailing := ""
	if r.trailingData && !r.textCut {
		n := r.lex.Read - r.endAt
		noun := "bytes follow"
		if n == 1 {
			noun = "byte follows"
		}
		trailing = fmt.Sprintf("%d %s the end of the document", n, noun)
	}
	const storeCut = "the text limit was reached; the stored data is cut off there"
	warnDoc := func() {
		if r.storeCut {
			r.doc.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, storeCut)
		}
		if trailing != "" {
			r.doc.Warn(gofastcore.SeverityInfo, gofastcore.WarnInputTrailingData, trailing)
		}
		r.cutWarnings(atStore, r.doc.Warn)
	}
	if len(r.store.Blocks) == 0 || !r.opts.Parts.Contains(2) || r.opts.Parts != (gofastcore.PartRange{}) {
		warnDoc()
		return
	}
	if m := r.opts.Limits.MaxParts; m > 0 && m < 2 {
		r.doc.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, "the part limit was reached; the stored part is left out")
		warnDoc()
		return
	}
	if r.storeCut {
		r.store.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded, storeCut)
	}
	if trailing != "" {
		r.store.Warn(gofastcore.SeverityInfo, gofastcore.WarnInputTrailingData, trailing)
	}
	r.cutWarnings(atStore, r.store.Warn)
	r.store.Finish()
	r.doc.Parts = append(r.doc.Parts, *r.store)
}
