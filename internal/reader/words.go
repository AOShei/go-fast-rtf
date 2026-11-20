package reader

import (
	"strings"

	"github.com/AOShei/go-fast-core"
	"github.com/AOShei/go-fast-rtf/internal/codepage"
	"github.com/AOShei/go-fast-rtf/internal/token"
)

// knownStar lists the destinations marked \* that this reader reads.
// Every other one is skipped, as the specification asks of a reader that
// does not know it.
var knownStar = map[string]bool{
	"fldinst": true, "generator": true, "docvar": true, "userprops": true,
	"revtbl": true, "listtable": true, "listoverridetable": true,
	"atnid": true, "atnauthor": true, "annotation": true, "atndate": true,
	"shpinst": true, "shppict": true, "picprop": true, "background": true,
	"objclass": true, "objname": true, "objdata": true, "bkmkstart": true,
	"nesttableprops": true, "pn": true, "ud": true, "template": true,
	"company": true, "manager": true, "category": true, "hlinkbase": true,
	"footnote": true, "header": true, "footer": true,
	"formfield": true, "ffname": true, "ffl": true,
}

// unreadStar names the skipped destinations that hold content a reader
// sees, so that a warning can say what was not read.
var unreadStar = map[string]string{
	"mmath": "equation", "moMathPara": "equation", "moMath": "equation",
}

var infoItems = map[string]bool{
	"title": true, "subject": true, "author": true, "operator": true, "keywords": true,
	"comment": true, "doccomm": true, "company": true, "manager": true, "category": true,
	"hlinkbase": true,
}

var specialChars = map[string]rune{
	"emdash": 0x2014, "endash": 0x2013, "bullet": 0x2022, "lquote": 0x2018, "rquote": 0x2019,
	"ldblquote": 0x201C, "rdblquote": 0x201D, "emspace": 0x2003, "enspace": 0x2002,
	"qmspace": 0x2005, "zwj": 0x200D, "zwnj": 0x200C, "ltrmark": 0x200E, "rtlmark": 0x200F,
	"zwbo": 0x200B, "zwnbo": 0xFEFF, "line": '\n', "softline": '\n', "lbr": '\n', "tab": '\t',
}

// word reads a control word.
func (r *reader) word(t token.Token) {
	g := r.top()
	name, v, has := t.Name, t.Param, t.HasParam
	r.flushBytes()
	first := g.first
	g.first = false
	r.firstWord = first
	if first && g.star && !knownStar[name] && !(g.dest == dStyleSheet && (name == "cs" || name == "ts" || name == "ds")) {
		if what, ok := unreadStar[name]; ok {
			r.unread[what]++
		}
		r.skipGroup()
		return
	}
	if r.ucLeft > 0 {
		r.ucLeft--
		return
	}
	on := !has || v != 0

	if c, ok := specialChars[name]; ok {
		if c == '\n' && g.buf == nil && g.flow == nil {
			return
		}
		r.put(c)
		return
	}

	switch g.dest {
	case dFontTbl:
		r.fontWord(name, v)
		return
	case dColorTbl:
		switch name {
		case "red":
			r.color.r, r.colorAny = uint8(min(max(v, 0), 255)), true
		case "green":
			r.color.g, r.colorAny = uint8(min(max(v, 0), 255)), true
		case "blue":
			r.color.b, r.colorAny = uint8(min(max(v, 0), 255)), true
		}
		return
	case dStyleSheet:
		if r.styleWord(g, name, v, has) {
			return
		}
	case dListTable, dListOverrideTable:
		r.listWord(g, name, v)
		return
	case dDate:
		if r.date != nil {
			r.date.set(name, v)
		}
		return
	case dPict:
		if r.pict != nil && r.pict.word(name, v) {
			return
		}
	case dInfo:
		if infoItems[name] {
			g.buf = r.infoBuf(name)
			return
		}
		switch name {
		case "creatim", "revtim", "printim", "buptim":
			g.dest = dDate
			r.date = &dateBuf{}
			r.dates[name] = r.date
			return
		}
	}

	if name == "box" || strings.Contains(name, "brdr") {
		borderWord(&g.p, name)
		return
	}

	switch name {
	// The document.
	case "ansi":
		r.ansicpg = 1252
	case "mac":
		r.ansicpg = 10000
	case "pc":
		r.ansicpg = 437
	case "pca":
		r.ansicpg = 850
	case "ansicpg":
		if has && v > 0 {
			r.ansicpg = int(v)
		}
	case "deff":
		r.deff = v
		if len(r.stack) > 0 {
			r.stack[0].c.font = v
			g.c.font = v
		}
	case "deflang":
		r.lang = v
	case "uc":
		g.uc = min(max(v, 0), 16)
	case "u":
		r.unicode(v)
	case "fonttbl":
		g.dest, g.buf = dFontTbl, nil
	case "colortbl":
		g.dest, g.buf = dColorTbl, nil
		r.colors, r.color, r.colorAny = r.colors[:0], rgb{}, false
	case "stylesheet":
		g.dest, g.buf = dStyleSheet, nil
	case "listtable":
		g.dest, g.buf = dListTable, nil
	case "listoverridetable":
		g.dest, g.buf = dListOverrideTable, nil
	case "info":
		g.dest, g.buf = dInfo, nil
	case "revtbl":
		g.dest, g.buf = dRevTbl, nil
	case "generator":
		if r.generator == nil {
			r.generator = newBuf(4096)
		}
		g.dest, g.buf = dBody, r.generator
	case "docvar":
		r.docVar(g)
	case "userprops":
		r.userProps(g)
	case "propname":
		if g.dest == dUserProps && len(r.props)+len(r.userprops) < maxStoredItems {
			r.props = append(r.props, [2]*strbuf{newBuf(1024), newBuf(1 << 16)})
			g.buf = r.props[len(r.props)-1][0]
		}
	case "staticval":
		if g.dest == dUserProps {
			if len(r.props) == 0 {
				r.props = append(r.props, [2]*strbuf{newBuf(1024), newBuf(1 << 16)})
			}
			g.buf = r.props[len(r.props)-1][1]
		}
	case "template":
		b := newBuf(2048)
		g.buf, g.dest = b, dBody
		g.onClose = func() {
			s := strings.ToLower(b.String())
			if strings.Contains(s, "://") || strings.HasPrefix(s, `\\`) {
				r.external++
			}
		}

	// Paragraphs.
	case "pard":
		g.p = defaultPara()
	case "plain":
		g.c = defaultChr()
		g.c.font = r.deff
	case "par":
		r.par(g)
	case "sect", "page", "column":
		if g.flow != nil && g.buf == nil && g.dest == dBody {
			if len(g.flow.text) > 0 {
				g.flow.endPara(g.p)
			}
			g.flow.pageBreak = true
		}
	case "s":
		g.p.style = v
		r.applyStyle(&g.c, v, false)
	case "cs":
		r.applyStyle(&g.c, v, true)
	case "outlinelevel":
		g.p.outline = v
	case "ls":
		g.p.ls = v
	case "ilvl":
		g.p.ilvl = v
	case "intbl":
		g.p.intbl = true
	case "itap":
		g.p.itap = max(v, 0)
	case "cbpat":
		g.p.shd = v
	case "listtext", "pntext":
		if g.flow != nil {
			g.flow.marker = newBuf(256)
			g.buf, g.dest = g.flow.marker, dBody
		} else {
			g.buf = newBuf(0)
		}
	case "pn":
		g.dest = dWords
	case "pnlvlblt":
		if g.flow != nil {
			g.flow.pnOrdered, g.flow.pnLevel = -1, 1
		}
	case "pnlvlbody":
		if g.flow != nil {
			g.flow.pnLevel = 1
		}
	case "pnlvl":
		if g.flow != nil {
			g.flow.pnLevel = min(max(v, 1), 9)
		}
	case "pndec", "pnucltr", "pnlcltr", "pnucrm", "pnlcrm", "pnord", "pncard", "pnordt":
		if g.flow != nil {
			g.flow.pnOrdered = 1
		}

	// Tables.
	case "cell", "nestcell":
		if g.flow != nil && g.buf == nil {
			level := 1
			if name == "nestcell" {
				level = max(int(min(g.p.itap, 64)), 2)
			}
			g.flow.endCell(g.p, level)
		}
	case "row", "nestrow":
		if g.flow != nil {
			level := 1
			if name == "nestrow" {
				level = max(int(min(g.p.itap, 64)), 2)
			}
			g.flow.endRow(level)
		}
	case "trowd", "cellx", "clmgf", "clmrg", "clvmgf", "clvmrg", "trhdr", "clcbpat":
		r.rowWord(g, name, v)
	case "nesttableprops":
		g.dest, g.nest = dWords, true
	case "nonesttables", "shprslt", "nonshppict", "ftnsep", "ftnsepc", "aftnsep", "aftnsepc", "ftncn", "aftncn":
		r.skipGroup()

	// Characters.
	case "b":
		g.c.bold = on
	case "i":
		g.c.italic = on
	case "ul", "uld", "uldash", "uldashd", "uldashdd", "uldb", "ulhwave", "ulldash", "ulth", "ulthd", "ulthdash", "ulthdashd", "ulthdashdd", "ulthldash", "ululdbwave", "ulw", "ulwave":
		g.c.ul = on
	case "ulnone":
		g.c.ul = false
	case "strike", "striked":
		g.c.strike = on
	case "super":
		g.c.sup, g.c.sub = true, false
	case "sub":
		g.c.sub, g.c.sup = true, false
	case "nosupersub":
		g.c.sup, g.c.sub = false, false
	case "v":
		g.c.hidden = on
	case "webhidden":
		g.c.webHidden = on
	case "fs":
		if has {
			g.c.fs = v
		} else {
			g.c.fs = 24
		}
	case "f":
		g.c.font = v
	case "cf":
		g.c.cf = v
	case "cb":
		g.c.cb = v
	case "highlight":
		g.c.hl = v
	case "chcbpat":
		g.c.shd = v
	case "deleted":
		g.c.deleted = on
	case "revised":
		if on {
			r.inserted++
		}
	case "revauthdel":
		g.c.revDel = v
	case "revdttmdel":
		g.c.dttmDel = v
	case "xe", "tc", "tcn":
		r.openEntry(g, name)

	// Notes.
	case "footnote":
		r.noteFlow(g, flowFootnote)
	case "ftnalt":
		if g.flow != nil && g.flow.kind == flowFootnote {
			g.flow.endnote = true
		}
	case "chftn":
		if g.flow != nil && g.flow.kind == flowFootnote {
			g.flow.autoMark = true
		}
	case "ftnnar", "ftnnalc", "ftnnauc", "ftnnrlc", "ftnnruc":
		r.ftnFmt = numberFormat(name[4:])
	case "aftnnar", "aftnnalc", "aftnnauc", "aftnnrlc", "aftnnruc":
		r.endFmt = numberFormat(name[5:])
	case "annotation":
		r.noteFlow(g, flowNote)
	case "atnid", "atnauthor":
		b := newBuf(1024)
		g.buf, g.dest = b, dBody
		g.onClose = func() {
			if name == "atnid" {
				r.atnID = cleanText(b.String())
			} else {
				r.atnAuthor = cleanText(b.String())
			}
		}
	case "atndate":
		b := newBuf(32)
		g.buf, g.dest = b, dBody
		f := g.flow
		g.onClose = func() {
			if f != nil && f.kind == flowNote {
				f.date = dttm(parseInt(b.String()))
			}
		}
	case "header", "headerl", "headerr", "headerf", "footer", "footerl", "footerr", "footerf":
		kind := flowHeader
		if strings.HasPrefix(name, "footer") {
			kind = flowFooter
		}
		f := &flow{r: r, kind: kind}
		g.flow, g.buf, g.dest, g.p = f, nil, dBody, defaultPara()
		g.onClose = f.end
	case "bkmkstart":
		b := newBuf(256)
		g.buf, g.dest = b, dBody
		f := g.flow
		g.onClose = func() {
			if f != nil && len(f.text) == 0 && f.anchor == "" {
				f.anchor = cleanText(b.String())
			}
		}

	// Fields.
	case "field":
		r.openField(g)
	case "fldinst":
		if n := len(r.fields); n > 0 {
			fl := r.fields[n-1]
			g.buf, g.dest = fl.instr, dBody
			g.onClose = func() { fl.href = fieldHref(fl.instr.String()) }
		} else {
			r.skipGroup()
		}
	case "fldrslt":
		if n := len(r.fields); n > 0 {
			fl := r.fields[n-1]
			if fl.inInstr {
				g.buf = newBuf(0)
			} else if r.opts.Spans && fl.href != "" {
				g.href = fl.href
			}
		}

	// Shapes, pictures and objects.
	case "shp", "shpgrp":
		r.openShape(g)
	case "shpinst", "shppict", "ud", "result":
	case "shptxt":
		f := &flow{r: r, kind: flowBox, parent: g.flow, back: r.shape.background()}
		g.flow, g.buf, g.dest, g.p = f, nil, dBody, defaultPara()
		g.onClose = f.end
	case "sp":
		r.openSp(g)
	case "sn", "sv":
		if r.sp != nil {
			g.buf = r.sp.name
			if name == "sv" {
				g.buf = r.sp.value
			}
		} else {
			g.buf = newBuf(0)
		}
		g.dest = dBody
	case "picprop":
		g.dest = dWords
	case "background":
		g.dest = dWords
		r.bgDepth = len(r.stack)
		g.onClose = func() { r.bgDepth = 0 }
	case "pict":
		r.openPict(g)
	case "object":
		r.openObject(g)
	case "objemb", "objlink", "objautlink", "objsub", "objpub", "objicemb", "objhtml", "objocx":
		if r.obj != nil {
			r.obj.kind = name
		}
	case "objclass", "objname":
		if r.obj != nil {
			g.buf = r.obj.class
			if name == "objname" {
				g.buf = r.obj.name
			}
			g.dest = dBody
		} else {
			r.skipGroup()
		}
	case "objdata":
		if r.obj != nil {
			g.dest = dObjData
			r.obj.start(r)
		} else {
			r.skipGroup()
		}
	case "upr":
		g.uprSkip = true
	case "formfield":
		r.openFormField(g)
	case "ffname", "ffl":
		if ff := r.form; ff != nil {
			b := newBuf(4096)
			g.buf, g.dest = b, dBody
			g.onClose = func() {
				s := cleanText(b.String())
				if name == "ffname" {
					ff.name = s
				} else if s != "" && len(ff.choices) < 1024 {
					ff.choices = append(ff.choices, s)
				}
			}
		} else {
			r.skipGroup()
		}
	}
}

// formField is the settings of a form field being read.
type formField struct {
	name    string
	choices []string
}

// openFormField reads \*\formfield: the name of a drop-down form field and
// the values it offers, which become a note of type choices.
func (r *reader) openFormField(g *group) {
	ff := &formField{}
	r.form = ff
	g.dest, g.buf = dWords, nil
	g.onClose = func() {
		r.form = nil
		if len(ff.choices) == 0 {
			return
		}
		b := gofastcore.Block{Kind: gofastcore.KindNote, Text: strings.Join(ff.choices, "\n"),
			Attrs: map[string]string{gofastcore.AttrNoteType: gofastcore.NoteChoices}}
		if ff.name != "" {
			b.Attrs[gofastcore.AttrField] = ff.name
		}
		r.deliver(r.top().flow, b)
	}
}

// borderWord reads a control word about borders. A side word (\brdrb,
// \box) says which sides the words after it are for; a style word
// (\brdrs, \brdrdb, ...) gives those sides a border, \brdrnone and
// \brdrnil take it away. The border words of cells, rows, pages and
// characters (\clbrdrb, \trbrdrb, \pgbrdrb, \chbrdr) end a paragraph's
// border words, so the style words after them are not the paragraph's.
func borderWord(p *para, name string) {
	switch name {
	case "brdrt":
		p.bpend = borderTop
	case "brdrl":
		p.bpend = borderLeft
	case "brdrr":
		p.bpend = borderRight
	case "brdrb":
		p.bpend = borderBottom
	case "brdrbtw":
		p.bpend = borderBetween
	case "brdrbar":
		p.bpend = borderBar
	case "box":
		p.bpend = borderTop | borderLeft | borderRight | borderBottom
	case "brdrnone", "brdrnil":
		p.bdr &^= p.bpend
	case "brdrw", "brdrcf", "brdrart", "brdrfr":
	default:
		if !strings.HasPrefix(name, "brdr") {
			p.bpend = 0
			return
		}
		p.bdr |= p.bpend
	}
}

// par ends a paragraph, or in a plain-text destination begins a new line.
func (r *reader) par(g *group) {
	switch {
	case g.buf != nil:
		g.buf.addRune('\n')
	case g.flow != nil && g.dest == dBody:
		g.flow.endPara(g.p)
	}
}

func numberFormat(s string) string {
	switch s {
	case "alc":
		return "alpha"
	case "auc":
		return "ALPHA"
	case "rlc":
		return "roman"
	case "ruc":
		return "ROMAN"
	}
	return "arabic"
}

func (r *reader) fontWord(name string, v int32) {
	switch name {
	case "f":
		r.curFont = v
		if _, ok := r.fonts[v]; !ok && len(r.fonts) < 1<<16 {
			r.fonts[v] = font{}
		}
	case "fcharset":
		f, ok := r.fonts[r.curFont]
		if ok && f.cpg == 0 {
			f.cp = codepage.ForCharset(int(v))
			r.fonts[r.curFont] = f
		}
	case "cpg":
		if f, ok := r.fonts[r.curFont]; ok && v > 0 {
			f.cp, f.cpg = int(v), int(v)
			r.fonts[r.curFont] = f
		}
	}
}

func (r *reader) endColor() {
	if len(r.colors) >= 1<<16 {
		return
	}
	c := r.color
	c.auto = !r.colorAny
	r.colors = append(r.colors, c)
	r.color, r.colorAny = rgb{}, false
}

// styleWord reads a control word of a style definition. It reports
// whether the word was taken.
func (r *reader) styleWord(g *group, name string, v int32, has bool) bool {
	st := g.style
	if st == nil {
		return true
	}
	on := v != 0 || !has
	switch name {
	case "s":
		st.num = v
	case "cs", "ds", "ts":
		st.num, st.char, st.cs = v, true, name == "cs"
	case "outlinelevel":
		st.outline = v
	case "sbasedon":
		st.basedOn = v
	case "v":
		st.set |= propHidden
		st.hidden = on
	case "webhidden":
		st.set |= propWebHidden
		st.webHidden = on
	case "cf":
		st.set |= propCf
		st.c.cf = v
	case "cb":
		st.set |= propCb
		st.c.cb = v
	case "highlight":
		st.set |= propHl
		st.c.hl = v
	case "chcbpat":
		st.set |= propShd
		st.c.shd = v
	case "fs":
		st.set |= propFs
		st.c.fs = 24
		if has {
			st.c.fs = v
		}
	}
	return true
}

// Character properties a style sets.
const (
	propHidden uint8 = 1 << iota
	propWebHidden
	propCf
	propCb
	propHl
	propShd
	propFs
)

// applyStyle gives c the character properties that style num sets, and
// those of the styles it is based on, the base first. A paragraph style
// (\sN) and a character style (\csN) are in two tables.
func (r *reader) applyStyle(c *chr, num int32, char bool) {
	table := r.styles
	if char {
		table = r.cstyles
	}
	var chain [16]*style
	n := 0
	for s := num; n < len(chain); n++ {
		st, ok := table[s]
		if !ok || st.char != char {
			break
		}
		chain[n] = st
		if st.basedOn < 0 || st.basedOn == s {
			n++
			break
		}
		s = st.basedOn
	}
	for i := n - 1; i >= 0; i-- {
		st := chain[i]
		if st == nil {
			continue
		}
		if st.set&propHidden != 0 {
			c.hidden = st.hidden
		}
		if st.set&propWebHidden != 0 {
			c.webHidden = st.webHidden
		}
		if st.set&propCf != 0 {
			c.cf = st.c.cf
		}
		if st.set&propCb != 0 {
			c.cb = st.c.cb
		}
		if st.set&propHl != 0 {
			c.hl = st.c.hl
		}
		if st.set&propShd != 0 {
			c.shd = st.c.shd
		}
		if st.set&propFs != 0 {
			c.fs = st.c.fs
		}
	}
}

func (r *reader) listWord(g *group, name string, v int32) {
	switch name {
	case "list":
		if g.dest != dListTable {
			return
		}
		l := &listDef{}
		r.curList = l
		g.onClose = func() {
			if l.hasID && len(r.lists) < 1<<16 {
				r.lists[l.id] = l.levels
			}
			r.curList = nil
		}
	case "listlevel":
		if l := r.curList; l != nil && len(l.levels) < 9 {
			l.levels = append(l.levels, 0)
			l.open = true
		} else if l != nil {
			l.open = false
		}
	case "levelnfc", "levelnfcn":
		if l := r.curList; l != nil && l.open && len(l.levels) > 0 {
			l.levels[len(l.levels)-1] = v
		}
	case "listid":
		if g.dest == dListTable && r.curList != nil {
			r.curList.id, r.curList.hasID = v, true
		} else if g.dest == dListOverrideTable && r.curOverride != nil {
			r.curOverride[0], r.curOverride[2] = v, 1
		}
	case "listoverride":
		o := &[3]int32{}
		r.curOverride = o
		g.onClose = func() {
			if o[2] == 1 && o[1] > 0 && len(r.lsList) < 1<<16 {
				r.lsList[o[1]] = o[0]
			}
			r.curOverride = nil
		}
	case "ls":
		if r.curOverride != nil {
			r.curOverride[1] = v
		}
	}
}

// rowWord reads the control words that define a table row.
func (r *reader) rowWord(g *group, name string, v int32) {
	f := g.flow
	if f == nil {
		return
	}
	level := 1
	if g.nest {
		level = max(int(min(g.p.itap, 64)), 2)
	}
	d := f.def(level)
	switch name {
	case "trowd":
		*d = rowDef{cells: d.cells[:0]}
	case "cellx":
		if len(d.cells) < 4096 {
			d.cells = append(d.cells, d.next)
		}
		d.next = cellDef{}
	case "clmgf":
		d.next.hm = 1
	case "clmrg":
		d.next.hm = 2
	case "clvmgf":
		d.next.vm = 1
	case "clvmrg":
		d.next.vm = 2
	case "clcbpat":
		d.next.shd = v
	case "trhdr":
		d.header = true
	}
}

// noteFlow makes the group a footnote or a comment.
func (r *reader) noteFlow(g *group, kind flowKind) {
	parent := g.flow
	f := &flow{r: r, kind: kind, parent: parent}
	if kind == flowNote {
		f.author = r.atnAuthor
		if f.author == "" {
			f.author = r.atnID
		}
		r.atnAuthor, r.atnID = "", ""
	}
	g.flow, g.buf, g.dest, g.p = f, nil, dBody, defaultPara()
	g.onClose = func() { r.endNote(f, parent) }
}

func (r *reader) endNote(f *flow, parent *flow) {
	text, spans, hidden, rest := f.merge()
	var out []gofastcore.Block
	if text != "" {
		b := gofastcore.Block{Text: text, Spans: spans, Hidden: hidden}
		if f.kind == flowNote {
			b.Kind = gofastcore.KindNote
			b.Attrs = map[string]string{gofastcore.AttrNoteType: gofastcore.NoteComment}
			if f.author != "" {
				b.Attrs[gofastcore.AttrAuthor] = f.author
			}
			if f.date != "" {
				b.Attrs[gofastcore.AttrDate] = f.date
			}
		} else {
			b.Kind = gofastcore.KindFootnote
		}
		out = append(out, b)
	}
	if f.kind == flowFootnote && f.autoMark {
		if f.endnote {
			r.endNum++
			if len(out) > 0 {
				out[0].Marker = formatNumber(r.endNum, r.endFmt)
			}
		} else {
			r.ftnNum++
			if len(out) > 0 {
				out[0].Marker = formatNumber(r.ftnNum, r.ftnFmt)
			}
		}
	}
	out = append(out, rest...)
	for _, b := range out {
		if parent != nil {
			r.holdBlock(&b)
			parent.after = append(parent.after, b)
		} else {
			r.add(b)
		}
	}
}

func (r *reader) docVar(g *group) {
	name, value := newBuf(1024), newBuf(1<<16)
	g.dest, g.buf = dDocVar, nil
	g.kids = []*strbuf{name, value}
	g.onClose = func() {
		n, v := cleanText(name.String()), cleanText(value.String())
		text := n + ": " + v
		switch {
		case n == "" && v == "":
			return
		case v == "":
			text = n
		case n == "":
			text = v
		}
		if len(r.docvars) < maxStoredItems {
			r.docvars = append(r.docvars, [2]string{text, n})
		}
	}
}

// maxStoredItems bounds the document variables, and the custom
// properties, that are kept.
const maxStoredItems = 1 << 14

func (r *reader) userProps(g *group) {
	g.dest, g.buf = dUserProps, nil
	r.props = r.props[:0]
	g.onClose = func() {
		for _, p := range r.props {
			n, v := cleanText(p[0].String()), cleanText(p[1].String())
			if n == "" && v == "" {
				continue
			}
			text := n + ": " + v
			if v == "" {
				text = n
			}
			if len(r.userprops) < maxStoredItems {
				r.userprops = append(r.userprops, [2]string{text, n})
			}
		}
		r.props = r.props[:0]
	}
}
